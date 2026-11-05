// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

// Package bond объединяет независимые надёжных каналов в один поток relay.
package bond

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"whitelist-bypass/internal/roommgr"
	"whitelist-bypass/relay/tunnel"
)

const (
	Lanes                = 10
	LaneConnID    uint32 = 0xfffffff0
	header               = 30
	signature            = sha256.Size
	maxPayload           = 65536
	maxReorder           = 16 << 20
	maxFrames            = 512
	probeSize            = 32768
	kindChallenge byte   = 1
	kindHello     byte   = 2
	kindAck       byte   = 3
	kindProbe     byte   = 4
	kindEcho      byte   = 5
	kindData      byte   = 6
	kindCredit    byte   = 7
	maxInFlight          = 192
)

type Epoch [16]byte

type Stats struct {
	Lane         int     `json:"канал"`
	Verified     bool    `json:"проверен"`
	Sent         uint64  `json:"отправлено_байт"`
	Received     uint64  `json:"получено_байт"`
	RTTMillis    float64 `json:"RTT_мс"`
	JitterMillis float64 `json:"джиттер_мс"`
	DeliveryMbps float64 `json:"доставка_Мбит_с"`
	Recovered    uint64  `json:"восстановлено_блоков"`
	Duplicates   uint64  `json:"отброшено_копий"`
	InFlight     uint64  `json:"ожидает_подтверждения_байт"`
}

type lane struct {
	raw                           tunnel.DataTunnel
	serial                        uint64
	jobs                          chan []byte
	controls                      chan []byte
	challenge                     [32]byte
	probe                         []byte
	authenticated, verified       bool
	queued                        int
	sent, received                uint64
	delivered                     uint64
	recovered, duplicates         uint64
	rtt, jitter, rate             float64
	lastReceipt, timeRate, pingAt time.Time
	rateBytes, pingSeq            uint64
	maxEnqueuedSeq                uint64
}

// Transport передаёт блоки одного TCP через разные каналы и восстанавливает порядок.
// Обычное объединение закрывается при потере канала. Режим 2+2 сохраняет поток
// и восстанавливает пропуски по сквозным номерам блоков через другие сессии.
type Transport struct {
	mu                         sync.Mutex
	sendMu                     sync.Mutex
	key                        [32]byte
	server                     bool
	epoch                      Epoch
	clientID                   [16]byte
	lanes                      [Lanes]lane
	pending                    map[uint64][]byte
	pendingBytes               int
	recvSeq, sendSeq, ackedSeq uint64
	cursor                     int
	ready, everReady           bool
	onData                     func([]byte)
	onClose                    func()
	onReady                    func()
	log                        func(string, ...any)
	stop                       chan struct{}
	readyCh                    chan struct{}
	wake                       chan struct{}
	recvWake                   chan struct{}
	once                       sync.Once
	err                        error
	born                       time.Time
	mode                       Mode
	modeSet                    bool
	samples                    map[uint64]sentSample
	pingCounter                uint64
	heartbeat                  uint64
	width                      int
	paired                     bool
	selective                  bool
	primaryDegraded            bool
	primaryEpochPinned         bool
	maxPrimaryReceiptSeq       uint64
	duplicateDelay             time.Duration
	outbound                   map[uint64]*pairedFrame
	onPeerActivity             func([16]byte)
	onRelease                  func([16]byte)
	released                   chan struct{}
	releaseOnce                sync.Once
	firstSendAt                time.Time
	lastGapSeq                 uint64
	lastGapAt                  time.Time
	faultUntil                 [Lanes]time.Time
}

func GroupKey(links []string) ([32]byte, error) {
	if len(links) < 1 || len(links) > Lanes {
		return [32]byte{}, fmt.Errorf("в группе нужны от одной до %d ссылок", Lanes)
	}
	seen := map[string]bool{}
	var b bytes.Buffer
	b.WriteString("telemost-bond-v1\n")
	for _, link := range links {
		s, e := roommgr.NormalizeLink(link)
		if e != nil {
			return [32]byte{}, e
		}
		if seen[s] {
			return [32]byte{}, errors.New("ссылки группы повторяются")
		}
		seen[s] = true
		b.WriteString(s)
		b.WriteByte('\n')
	}
	return sha256.Sum256(b.Bytes()), nil
}

func New(key [32]byte, server bool, log func(string, ...any), widths ...int) *Transport {
	return newTransport(key, server, log, false, 0, widths...)
}

func newTransport(key [32]byte, server bool, log func(string, ...any), paired bool, delay time.Duration, widths ...int) *Transport {
	width := Lanes
	if len(widths) > 0 {
		width = widths[0]
	}
	if width < 1 || width > Lanes {
		panic("неверное число каналов")
	}

	t := &Transport{key: key, server: server, log: log, stop: make(chan struct{}), readyCh: make(chan struct{}), wake: make(chan struct{}, 1), recvWake: make(chan struct{}, 1), pending: map[uint64][]byte{}, recvSeq: 1, sendSeq: 1, born: time.Now()}
	t.width = width
	t.paired = paired
	t.selective = paired && width == 2
	t.duplicateDelay = delay
	if paired {
		t.outbound = make(map[uint64]*pairedFrame)
		t.released = make(chan struct{})
	}
	t.mode = Balanced
	t.samples = make(map[uint64]sentSample)
	if server {
		mustRandom(t.epoch[:])
	} else {
		mustRandom(t.clientID[:])
	}
	for i := range t.lanes[:t.width] {
		t.lanes[i].jobs = make(chan []byte, 16)
		t.lanes[i].controls = make(chan []byte, 512)
		go t.writer(i)
	}
	go t.reader()
	go t.handshakes()
	if paired {
		go t.redundancy()
	}
	return t
}

func mustRandom(b []byte) {
	if _, e := rand.Read(b); e != nil {
		panic("недоступен криптографический генератор")
	}
}
func notify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
func (t *Transport) Done() <-chan struct{}  { return t.stop }
func (t *Transport) Ready() <-chan struct{} { return t.readyCh }
func (t *Transport) Err() error             { t.mu.Lock(); defer t.mu.Unlock(); return t.err }
func (t *Transport) SetOnData(fn func([]byte)) {
	t.mu.Lock()
	t.onData = fn
	t.mu.Unlock()
	notify(t.recvWake)
}
func (t *Transport) SetOnClose(fn func()) {
	t.mu.Lock()
	t.onClose = fn
	closed := t.err != nil
	t.mu.Unlock()
	if closed && fn != nil {
		fn()
	}
}
func (t *Transport) SetOnReady(fn func()) {
	t.mu.Lock()
	t.onReady = fn
	ready := t.ready
	t.mu.Unlock()
	if ready && fn != nil {
		fn()
	}
}
func (t *Transport) Reconfigure(int, int) {}
func (t *Transport) Stats() []Stats {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := make([]Stats, t.width)
	for i := range t.lanes[:t.width] {
		l := &t.lanes[i]
		s[i] = Stats{Lane: i + 1, Verified: l.verified, Sent: l.sent, Received: l.received, RTTMillis: l.rtt * 1000, JitterMillis: l.jitter * 1000, DeliveryMbps: l.rate * 8 / 1e6, Recovered: l.recovered, Duplicates: l.duplicates, InFlight: l.sent - l.delivered}
	}
	return s
}

func (t *Transport) Close() {
	t.fail(errors.New("объединённая сессия закрыта"))
}
func (t *Transport) fail(e error) {
	var fn func()
	first := false
	t.once.Do(func() {
		t.mu.Lock()
		t.err = e
		t.ready = false
		t.pending = map[uint64][]byte{}
		t.pendingBytes = 0
		t.outbound = nil
		fn = t.onClose
		close(t.stop)
		t.mu.Unlock()
		first = true
	})
	if first {
		if t.log != nil {
			t.log("Объединение: %v", e)
		}
		if fn != nil {
			fn()
		}
	}
}

// Attach не устанавливает обработчик данных: адаптер сохраняет сообщения Config/ConfigAck.
func (t *Transport) Attach(i int, raw tunnel.DataTunnel) {
	if i < 0 || i >= t.width {
		return
	}
	t.mu.Lock()
	if t.err != nil {
		t.mu.Unlock()
		return
	}
	l := &t.lanes[i]
	if l.raw != nil && t.everReady && !t.paired {
		t.mu.Unlock()
		t.fail(errors.New("канал заменён; текущие TCP закрыты"))
		return
	}
	l.raw = raw
	l.serial++
	l.authenticated = false
	l.verified = false
	l.probe = nil
	mustRandom(l.challenge[:])
	t.mu.Unlock()
	t.handshake(i)
}
func (t *Transport) Detach(i int, raw tunnel.DataTunnel) {
	if i < 0 || i >= t.width {
		return
	}
	t.mu.Lock()
	l := &t.lanes[i]
	same := l.raw == raw
	if same {
		l.raw = nil
		l.serial++
		l.authenticated = false
		l.verified = false
	}
	used := t.everReady
	t.mu.Unlock()
	notify(t.wake)
	if same && used && !t.paired {
		t.fail(errors.New("потерян канал; текущие TCP закрыты"))
	}
}

func (t *Transport) encode(kind byte, i int, seq uint64, body []byte) []byte {
	p := make([]byte, header+len(body)+signature)
	copy(p, t.magic())
	p[4] = kind
	p[5] = byte(i)
	copy(p[6:22], t.epoch[:])
	binary.BigEndian.PutUint64(p[22:30], seq)
	copy(p[header:], body)
	m := hmac.New(sha256.New, t.key[:])
	m.Write(p[:len(p)-signature])
	copy(p[len(p)-signature:], m.Sum(nil))
	return p
}
func (t *Transport) queueControl(i int, p []byte) {
	select {
	case t.lanes[i].controls <- p:
	case <-t.stop:
	default:
	}
}
func (t *Transport) handshake(i int) {
	t.mu.Lock()
	if t.err != nil || t.lanes[i].raw == nil {
		t.mu.Unlock()
		return
	}
	l := &t.lanes[i]
	var p []byte
	if t.server {
		if !l.verified {
			p = t.encode(kindChallenge, i, 0, l.challenge[:])
		}
	} else if t.epoch != (Epoch{}) {
		if !l.authenticated {
			body := append(append([]byte{}, t.clientID[:]...), l.challenge[:]...)
			p = t.encode(kindHello, i, 0, append(body, byte(t.mode)))
		} else if !l.verified {
			p = t.encode(kindProbe, i, 0, l.probe)
		}
	}
	t.mu.Unlock()
	if p != nil {
		t.queueControl(i, p)
	}
}
func (t *Transport) handshakes() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-t.stop:
			return
		case <-ticker.C:
		}
		t.mu.Lock()
		expired := !t.ready && time.Since(t.born) > 150*time.Second
		t.heartbeat++
		probeNow := t.heartbeat%5 == 0
		t.mu.Unlock()
		if expired {
			t.fail(errors.New("не все каналы прошли проверку за 150 секунд"))
			return
		}
		for i := 0; i < t.width; i++ {
			t.handshake(i)
		}
		t.sendCredit()
		if probeNow {
			t.qualityProbes()
		}
	}
}

func (t *Transport) checkReadyLocked() func() {
	if t.ready {
		return nil
	}
	for i := range t.lanes[:t.width] {
		if !t.lanes[i].verified {
			return nil
		}
	}
	t.ready = true
	t.everReady = true
	close(t.readyCh)
	return t.onReady
}

func (t *Transport) Handle(i int, raw tunnel.DataTunnel, p []byte) {
	if tunnel.LooksLikeRelayFrame(p) {
		tunnel.DecodeFrames(p, func(id uint32, kind byte, payload []byte) {
			if id == LaneConnID && kind == tunnel.MsgData {
				t.Handle(i, raw, payload)
			}
		})
		return
	}

	if i < 0 || i >= t.width || len(p) < header+signature || len(p) > header+maxPayload+signature || string(p[:4]) != t.magic() || int(p[5]) != i {
		return
	}
	m := hmac.New(sha256.New, t.key[:])
	m.Write(p[:len(p)-signature])
	if !hmac.Equal(p[len(p)-signature:], m.Sum(nil)) {
		return
	}
	kind := p[4]
	var epoch Epoch
	copy(epoch[:], p[6:22])
	seq := binary.BigEndian.Uint64(p[22:30])
	body := p[header : len(p)-signature]
	t.mu.Lock()
	if t.err != nil || t.lanes[i].raw != raw || time.Now().Before(t.faultUntil[i]) {
		t.mu.Unlock()
		return
	}
	l := &t.lanes[i]
	var reply []byte
	replyLane := i
	var fail error
	var becameReady func()
	var release func([16]byte)
	verifiedNow := false
	if kind == kindChallenge && !t.server && len(body) == 32 {
		if epoch != t.epoch {
			if t.selective && t.primaryEpochPinned && (!t.everReady || i != 0) {
				// Запоздалый вызов из старой резервной сессии не отменяет
				// уже подтверждённую основную эпоху. При реальном рестарте
				// до готовности внешний таймер начнёт новую попытку без TCP.
				t.mu.Unlock()
				return
			}
			if t.everReady {
				fail = errors.New("сервер сменил сессию; TCP закрыты")
			} else {
				t.epoch = epoch
				for j := range t.lanes[:t.width] {
					t.lanes[j].authenticated = false
					t.lanes[j].verified = false
					t.lanes[j].probe = nil
					t.lanes[j].challenge = [32]byte{}
				}
			}
		}
		if fail == nil {
			if !bytes.Equal(l.challenge[:], body) {
				copy(l.challenge[:], body)
				l.authenticated = false
				l.verified = false
				l.probe = nil
			}
			reply = t.encode(kindHello, i, 0, append(append(append([]byte{}, t.clientID[:]...), body...), byte(t.mode)))
		}
	} else if epoch == t.epoch {
		switch kind {
		case kindHello:
			if t.paired && t.server && t.onPeerActivity != nil && t.clientID == ([16]byte{}) {
				break
			}
			if t.server && len(body) == 49 && body[48] <= byte(Stability) && bytes.Equal(body[16:48], l.challenge[:]) {
				var id [16]byte
				copy(id[:], body[:16])
				if t.clientID != ([16]byte{}) && t.clientID != id {
					if !t.paired {
						fail = errors.New("новый клиент запросил новую сессию")
					}
				} else {
					if t.modeSet && t.mode != Mode(body[48]) {
						t.mu.Unlock()
						t.fail(errors.New("режимы каналов группы не совпадают"))
						return
					}
					t.mode = Mode(body[48])
					t.modeSet = true
					t.clientID = id
					l.authenticated = true
					reply = t.encode(kindAck, i, 0, body)
				}
			}
		case kindAck:
			if !t.server && len(body) == 49 && body[48] == byte(t.mode) && bytes.Equal(body[:16], t.clientID[:]) && bytes.Equal(body[16:48], l.challenge[:]) {
				if !l.authenticated {
					l.probe = make([]byte, probeSize)
					mustRandom(l.probe)
					l.authenticated = true
				}
				if !l.verified {
					reply = t.encode(kindProbe, i, 0, l.probe)
				}
			}
		case kindProbe:
			if t.server && l.authenticated && len(body) == probeSize {
				reply = t.encode(kindEcho, i, 0, body)
				l.verified = true
				becameReady = t.checkReadyLocked()
			}
		case kindEcho:
			if !t.server && l.authenticated && !l.verified && len(l.probe) == probeSize && bytes.Equal(body, l.probe) {
				l.verified = true
				l.probe = nil
				if t.selective && i == 0 {
					t.primaryEpochPinned = true
				}
				verifiedNow = true
				becameReady = t.checkReadyLocked()
			}
		case kindGap:
			if t.selective && l.authenticated && l.verified && seq > t.ackedSeq && seq < t.sendSeq {
				if f := t.outbound[seq]; f != nil {
					f.requested = true
				}
			}
		case kindReceipt:
			if l.authenticated && l.verified {
				receivedLane := i
				freshReceipt := false
				valid := true
				if t.selective {
					if len(body) != 10 || int(body[0]) >= t.width {
						valid = false
					} else {
						receivedLane = int(body[0])
						freshReceipt = body[1] == 1
						body = body[2:]
					}
				}
				if valid {
					t.receiptLocked(receivedLane, seq, body)
					if t.selective && receivedLane == 0 {
						t.primaryDegraded = false
						if seq > t.maxPrimaryReceiptSeq && seq < t.sendSeq {
							t.maxPrimaryReceiptSeq = seq
						}
					}
					if t.paired && len(body) == 8 && binary.BigEndian.Uint64(body) <= t.lanes[receivedLane].sent {
						if f := t.outbound[seq]; f != nil {
							if t.selective && receivedLane == 1 && freshReceipt && f.firstLane == 0 && !f.received && seq > t.maxPrimaryReceiptSeq {
								t.primaryDegraded = true
							}
							f.received = true
						}
					}
				}
			}
		case kindPing:
			if l.authenticated && l.verified {
				reply = t.encode(kindPong, i, seq, nil)
			}
		case kindPong:
			if l.authenticated && l.verified && seq == l.pingSeq && !l.pingAt.IsZero() {
				l.sampleRTT(time.Since(l.pingAt).Seconds())
				if t.selective && i == 0 {
					t.primaryDegraded = false
				}
				l.pingAt = time.Time{}
			}
		case kindRelease:
			if t.paired && t.server && l.authenticated && l.verified {
				reply = t.encode(kindReleased, i, 0, nil)
				if t.onRelease != nil {
					release = t.onRelease
				}
			}
		case kindReleased:
			if t.paired && !t.server && l.authenticated && l.verified {
				t.releaseOnce.Do(func() { close(t.released) })
			}
		case kindCredit:
			if l.authenticated && l.verified && seq > t.ackedSeq && seq < t.sendSeq {
				t.ackedSeq = seq
				notify(t.wake)
			}
		case kindData:
			if l.authenticated && l.verified {
				_, exists := t.pending[seq]
				fresh := seq >= t.recvSeq && !exists
				if seq < t.recvSeq || exists {
					if t.paired {
						l.duplicates++
					}
				} else if seq-t.recvSeq >= maxFrames || t.pendingBytes+len(body) > maxReorder {
					fail = errors.New("превышен предел восстановления порядка; TCP закрыты")
				} else {
					t.pending[seq] = append([]byte{}, body...)
					t.pendingBytes += len(body)
					if t.paired && ((!t.selective && i >= 2) || (t.selective && i == 1)) {
						l.recovered++
					}
					notify(t.recvWake)
				}
				// Каждая полученная копия подтверждается, даже если первая уже доставлена.
				// Иначе потерянное подтверждение вызовет лишние повторные передачи.
				if fail == nil && seq > 0 {
					l.received += uint64(len(body))
					var total [8]byte
					binary.BigEndian.PutUint64(total[:], l.received)
					if t.selective {
						if t.lanes[1].verified {
							replyLane = 1
						}
						flag := byte(0)
						if fresh {
							flag = 1
						}
						reply = t.encode(kindReceipt, replyLane, seq, append([]byte{byte(i), flag}, total[:]...))
						if seq > t.recvSeq {
							t.requestGapLocked()
						}
					} else {
						reply = t.encode(kindReceipt, i, seq, total[:])
					}
				}
			}
		}
	}
	var activity func([16]byte)
	owner := t.clientID
	if t.paired && t.server && epoch == t.epoch && l.authenticated && l.verified && fail == nil {
		activity = t.onPeerActivity
	}
	t.mu.Unlock()
	if activity != nil {
		activity(owner)
	}
	if reply != nil {
		t.queueControl(replyLane, reply)
	}
	if release != nil {
		release(owner)
	}
	if verifiedNow && t.log != nil {
		t.log("Канал %d: двусторонняя проверка %d байт пройдена", i+1, probeSize)
	}
	if becameReady != nil {
		becameReady()
	}
	if fail != nil {
		t.fail(fail)
	}
}

func (t *Transport) writer(i int) {
	for {
		var p []byte
		// Подтверждения не стоят за очередью полезных данных. Иначе насыщение
		// здорового пути выглядит как потеря и создаёт ненужные резервные повторы.
		select {
		case <-t.stop:
			return
		case p = <-t.lanes[i].controls:
		default:
			select {
			case <-t.stop:
				return
			case p = <-t.lanes[i].controls:
			case p = <-t.lanes[i].jobs:
			}
		}
		t.mu.Lock()
		l := &t.lanes[i]
		raw := l.raw
		valid := t.err == nil && !time.Now().Before(t.faultUntil[i]) && bytes.Equal(p[6:22], t.epoch[:])
		t.mu.Unlock()
		if raw != nil && valid {
			raw.SendData(tunnel.EncodeFrame(LaneConnID, tunnel.MsgData, p))
		}
		if p[4] == kindData {
			t.mu.Lock()
			l.queued -= len(p)
			t.mu.Unlock()
			notify(t.wake)
		}
	}
}
func (t *Transport) reader() {
	for {
		select {
		case <-t.stop:
			return
		case <-t.recvWake:
		}
		for {
			t.mu.Lock()
			if t.err != nil {
				t.mu.Unlock()
				return
			}
			p, ok := t.pending[t.recvSeq]
			fn := t.onData
			if !ok || fn == nil {
				t.mu.Unlock()
				break
			}
			delete(t.pending, t.recvSeq)
			t.recvSeq++
			t.pendingBytes -= len(p)
			t.mu.Unlock()
			fn(p)
			t.mu.Lock()
			ack := t.recvSeq%16 == 0 || t.selective
			t.mu.Unlock()
			if ack {
				t.sendCredit()
			}
		}
	}
}

// Сквозные подтверждения ограничивают общий объём данных между каналами.
// Быстрые каналы не могут бесконечно обгонять потерянный блок медленного.
func (t *Transport) sendCredit() {
	t.mu.Lock()
	if !t.ready || t.err != nil {
		t.mu.Unlock()
		return
	}
	// При скрытой потере основного пути подтверждение нельзя отправлять
	// только по нему: резерв доставит данные, но окно отправителя иначе застынет.
	if t.paired {
		for i := 0; i < t.width; i++ {
			if t.lanes[i].verified {
				t.queueControl(i, t.encode(kindCredit, i, t.recvSeq-1, nil))
			}
		}
		t.mu.Unlock()
		return
	}
	best := -1
	for i := range t.lanes[:t.width] {
		if t.lanes[i].verified && (best < 0 || len(t.lanes[i].jobs) < len(t.lanes[best].jobs)) {
			best = i
		}
	}
	if best < 0 {
		t.mu.Unlock()
		return
	}
	p := t.encode(kindCredit, best, t.recvSeq-1, nil)
	t.mu.Unlock()
	t.queueControl(best, p)
}

func (t *Transport) SendData(data []byte) {
	if len(data) == 0 {
		return
	}
	if len(data) > maxPayload {
		t.fail(errors.New("слишком большой блок relay"))
		return
	}
	t.sendMu.Lock()
	defer t.sendMu.Unlock()
	for {
		t.mu.Lock()
		if t.err != nil || !t.ready {
			t.mu.Unlock()
			return
		}
		window := t.mode.window()
		if t.selective {
			window = t.selectiveWindow()
		}
		if t.sendSeq-t.ackedSeq > window || (t.paired && len(t.outbound) >= maxFrames) {
			t.mu.Unlock()
			select {
			case <-t.stop:
				return
			case <-t.wake:
			}
			continue
		}
		selected := t.chooseLaneLocked(len(data))
		if t.paired && selected < 0 {
			selected = t.chooseReserveLocked()
		}
		if selected >= 0 {
			i := selected
			l := &t.lanes[i]
			p := t.encode(kindData, i, t.sendSeq, data)
			select {
			case l.jobs <- p:
				l.queued += len(p)
				if t.sendSeq > l.maxEnqueuedSeq {
					l.maxEnqueuedSeq = t.sendSeq
				}
				l.sent += uint64(len(data))
				t.samples[t.sendSeq] = sentSample{at: time.Now(), lane: i}
				if t.sendSeq > maxFrames {
					delete(t.samples, t.sendSeq-maxFrames)
				}
				if t.paired {
					now := time.Now()
					if t.firstSendAt.IsZero() {
						t.firstSendAt = now
					}
					t.outbound[t.sendSeq] = &pairedFrame{body: append([]byte{}, data...), firstLane: i, copyDue: now.Add(t.duplicateDelay), lastRetry: now}
				}
				t.sendSeq++
				t.cursor = (i + 1) % t.width
				t.mu.Unlock()
				return
			default:
			}
		}
		t.mu.Unlock()
		select {
		case <-t.stop:
			return
		case <-t.wake:
		case <-time.After(10 * time.Millisecond):
		}
	}
}

var _ tunnel.DataTunnel = (*Transport)(nil)
