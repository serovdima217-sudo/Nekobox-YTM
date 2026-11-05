// Copyright (c) 2024 kulikov0
//
// Licensed under the MIT License.
// Полный текст: LICENSE.upstream-MIT.txt. Изменения Nekobox YTM: GPL-3.0-or-later.

package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"whitelist-bypass/relay/common"
	tmapi "whitelist-bypass/relay/telemost"
	"whitelist-bypass/relay/tunnel"
	"whitelist-bypass/relay/tunnel/rtc"

	"github.com/kulikov0/headless-client/webrtc"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
)

const (
	sharingRTPMTU     = 1200
	sharingVP8PT      = 96
	sharingClockRate  = 90000
	sharingStreamSSRC = 0x1a2b3c4d
	defaultSharingFPS = 24
)

type SFURelay struct {
	compact        bool
	cameraOnly     bool
	closed         bool
	duplicateDelay time.Duration
	pubPC          *webrtc.PeerConnection
	subPC          *webrtc.PeerConnection
	pubRemoteSet   bool
	subRemoteSet   bool
	pubPending     []webrtc.ICECandidateInit
	subPending     []webrtc.ICECandidateInit
	mu             sync.Mutex

	sampleTrack   *webrtc.TrackLocalStaticSample
	tun           *rtc.VP8DataTunnel
	mt            *rtc.MultiTrackTunnel
	delivered     tunnel.DataTunnel
	obf           *tunnel.TunnelObfuscator
	OnConnected   func(tunnel.DataTunnel)
	OnPubReady    func()
	OnPeerRestart func()
	OnPubICE      func(*webrtc.ICECandidate)
	OnSubICE      func(*webrtc.ICECandidate)

	sharingDC *webrtc.DataChannel

	readBufSize            int
	tunFired               bool
	cameraFPS, cameraBatch int
}

type sharingDCSink struct {
	dc         *webrtc.DataChannel
	mu         sync.Mutex
	packetizer rtp.Packetizer
	samples    uint32
	remainder  int64
}

func (r *SFURelay) SetObfuscator(o *tunnel.TunnelObfuscator) { r.obf = o }

func NewSFURelay() *SFURelay {
	return &SFURelay{}
}

func newSharingDCSink(dc *webrtc.DataChannel, fps int) *sharingDCSink {
	if fps <= 0 {
		fps = defaultSharingFPS
	}
	return &sharingDCSink{
		dc: dc,
		packetizer: rtp.NewPacketizer(sharingRTPMTU, sharingVP8PT, sharingStreamSSRC,
			&codecs.VP8Payloader{EnablePictureID: true}, rtp.NewRandomSequencer(), sharingClockRate),
		samples: uint32(sharingClockRate / fps),
	}
}

func (s *sharingDCSink) sendFrame(frame []byte) error { return s.sendTimedFrame(frame, 0) }

// Дробная часть сохраняется, чтобы часы 90 кГц не дрейфовали при высокой частоте.
func sharingSamples(duration time.Duration, remainder *int64) uint32 {
	n := int64(duration)*sharingClockRate + *remainder
	samples := n / int64(time.Second)
	*remainder = n % int64(time.Second)
	if samples < 1 {
		samples = 1
	}
	return uint32(samples)
}

func (s *sharingDCSink) sendTimedFrame(frame []byte, duration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dc.BufferedAmount() > 4<<20 {
		return nil
	}
	samples := s.samples
	if duration > 0 {
		samples = sharingSamples(duration, &s.remainder)
	}
	for _, pkt := range s.packetizer.Packetize(frame, samples) {
		raw, err := pkt.Marshal()
		if err != nil {
			continue
		}
		if err := s.dc.Send(raw); err != nil {
			return err
		}
	}
	return nil
}

func (r *SFURelay) AddSharingDataChannel() error {
	if r.cameraOnly {
		return fmt.Errorf("демонстрация экрана запрещена в режиме 2+2")
	}
	if r.pubPC == nil {
		return fmt.Errorf("pub PC nil")
	}
	unordered := false
	retries := uint16(0)
	dc, err := r.pubPC.CreateDataChannel("sharing", &webrtc.DataChannelInit{Ordered: &unordered, MaxRetransmits: &retries})
	if err != nil {
		return err
	}
	r.sharingDC = dc
	sink := newSharingDCSink(dc, defaultSharingFPS)
	keyframe := func() []byte {
		return r.obf.EncodeKeepalive(0)
	}
	dc.OnOpen(func() {
		log.Printf("[ss] creator 'sharing' DC open, registering screenshare sub-tunnel")
		sink.sendFrame(keyframe())
		sub := rtc.NewVP8DataTunnelWithQueue(nil, r.obf, log.Printf, rtc.KCPCarrierQueueDepth)
		sub.WriteTimedFrame = sink.sendTimedFrame
		r.mu.Lock()
		mt, cam, closed := r.mt, r.tun, r.closed
		r.mu.Unlock()
		if closed || mt == nil || cam == nil {
			return
		}
		sub.Start(cam.FPS(), cam.Batch())
		mt.AddSubTunnel(sub)
		log.Printf("[ss] screenshare sub-tunnel live, tracks=%d", mt.SubTunnelCount())
	})
	dc.OnClose(func() { log.Printf("[ss] creator 'sharing' DC closed") })
	dc.OnError(func(e error) { log.Printf("[ss] creator 'sharing' DC error: %v", e) })
	dc.OnMessage(func(m webrtc.DataChannelMessage) {
		if len(m.Data) >= 2 && m.Data[0]&0xC0 == 0x80 && m.Data[1] >= 200 && m.Data[1] <= 206 {
			if m.Data[1] == 206 {
				sink.sendFrame(keyframe())
			}
		}
	})
	return nil
}

func (r *SFURelay) RemoveSharingDataChannel() {
	if r.mt != nil {
		r.mt.RemoveLastSubTunnel()
	}
	if r.sharingDC != nil {
		r.sharingDC.Close()
		r.sharingDC = nil
	}
}

func (r *SFURelay) Init(iceServers []webrtc.ICEServer) error {
	config := webrtc.Configuration{ICEServers: iceServers}

	factory := tmapi.NewPeerConnection
	if r.compact {
		factory = tmapi.NewCompactPeerConnection
	}
	pubPC, err := factory(config)
	if err != nil {
		return err
	}
	r.pubPC = pubPC

	sampleTrack, _ := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
	)
	r.sampleTrack = sampleTrack

	audioTrack, _ := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
	)
	pubPC.AddTransceiverFromTrack(audioTrack, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly})
	videoTr, _ := pubPC.AddTransceiverFromTrack(sampleTrack, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly})
	if videoTr != nil {
		go rtc.DrainSenderRTCP(videoTr.Sender())
	}

	pubPC.OnICECandidate(func(cand *webrtc.ICECandidate) {
		if cand == nil || r.OnPubICE == nil {
			return
		}
		r.OnPubICE(cand)
	})

	pubPC.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("[pub] connection state: %s", state.String())
		if state == webrtc.PeerConnectionStateConnected {
			r.mu.Lock()
			if r.closed {
				r.mu.Unlock()
				return
			}
			if r.tun == nil {
				log.Println("[relay] starting VP8 publish tunnel on pub PC connected")
				r.tun = rtc.NewVP8DataTunnelWithQueue(r.sampleTrack, r.obf, log.Printf, rtc.KCPCarrierQueueDepth)
				if r.cameraOnly {
					if r.cameraFPS > 0 {
						r.tun.Start(r.cameraFPS, r.cameraBatch)
					} else {
						r.tun.Start(12, 40)
					}
				} else {
					r.tun.Start(0, 0)
				}
				r.tunFired = false
				r.mt = rtc.NewMultiTrackTunnel([]*rtc.VP8DataTunnel{r.tun})
				if r.duplicateDelay > 0 {
					r.mt.EnableDelayedDuplicate(r.duplicateDelay)
				}
				mt := r.mt
				r.mt.SetOnData(func(payload []byte) { r.activate(mt, payload) })
			}
			r.mu.Unlock()
			if r.OnPubReady != nil {
				r.OnPubReady()
			}
		}
	})

	subPC, err := factory(config)
	if err != nil {
		pubPC.Close()
		return err
	}
	r.subPC = subPC

	subPC.OnICECandidate(func(cand *webrtc.ICECandidate) {
		if cand == nil || r.OnSubICE == nil {
			return
		}
		r.OnSubICE(cand)
	})

	subPC.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("[sub] connection state: %s", state.String())
	})

	subPC.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("[sub] remote track: %s", track.Codec().MimeType)
		go r.readTrack(track)
	})

	log.Printf("[relay] pub+sub PCs created (%d ICE servers)", len(iceServers))
	return nil
}

func (r *SFURelay) activate(mt *rtc.MultiTrackTunnel, payload []byte) {
	r.mu.Lock()
	if r.tunFired || r.closed {
		r.mu.Unlock()
		return
	}
	r.tunFired = true
	r.mu.Unlock()

	var delivered tunnel.DataTunnel = mt
	useKCP := false
	if !tunnel.LooksLikeRelayFrame(payload) {
		delivered = rtc.NewMultiTrackKCPTunnel(mt, log.Printf)
		useKCP = true
		log.Println("[relay] per-track kcp reliability active over video tunnel")
	}
	log.Printf("[relay] auto-detected active tunnel: %T", delivered)
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		if k, ok := delivered.(*rtc.MultiTrackKCPTunnel); ok {
			k.StopLayer()
		}
		return
	}
	r.delivered = delivered
	r.mu.Unlock()
	if r.OnConnected != nil {
		r.OnConnected(delivered)
	}
	if useKCP {
		if kcptun, ok := delivered.(*rtc.MultiTrackKCPTunnel); ok {
			kcptun.InjectSegment(payload)
		}
	} else {
		mt.DeliverData(payload)
	}
}

func (r *SFURelay) resetForNewPeer() {
	r.mu.Lock()
	r.tunFired = false
	mt := r.mt
	old := r.delivered
	r.delivered = nil
	r.mu.Unlock()
	if kcp, ok := old.(*rtc.MultiTrackKCPTunnel); ok {
		kcp.StopLayer()
	}
	if mt != nil {
		mt.SetOnData(func(payload []byte) { r.activate(mt, payload) })
	}
}

func (r *SFURelay) CreatePubOffer() (webrtc.SessionDescription, error) {
	offer, err := r.pubPC.CreateOffer(nil)
	if err != nil {
		return offer, err
	}
	if err := r.pubPC.SetLocalDescription(offer); err != nil {
		return offer, err
	}
	offer.SDP = tmapi.MungeSDPAddVideoContent(offer.SDP)
	return offer, nil
}

func (r *SFURelay) SetPubAnswer(sdp string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	err := r.pubPC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer, SDP: sdp,
	})
	if err != nil {
		return err
	}
	r.pubRemoteSet = true
	for _, cand := range r.pubPending {
		r.pubPC.AddICECandidate(cand)
	}
	r.pubPending = nil
	return nil
}

func (r *SFURelay) SetSubOffer(sdp string) (webrtc.SessionDescription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	err := r.subPC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer, SDP: sdp,
	})
	if err != nil {
		return webrtc.SessionDescription{}, err
	}
	r.subRemoteSet = true
	for _, cand := range r.subPending {
		r.subPC.AddICECandidate(cand)
	}
	r.subPending = nil

	answer, err := r.subPC.CreateAnswer(nil)
	if err != nil {
		return answer, err
	}
	r.subPC.SetLocalDescription(answer)
	return answer, nil
}

func (r *SFURelay) AddPubICECandidate(cand webrtc.ICECandidateInit) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.pubRemoteSet {
		r.pubPending = append(r.pubPending, cand)
		return
	}
	r.pubPC.AddICECandidate(cand)
}

func (r *SFURelay) AddSubICECandidate(cand webrtc.ICECandidateInit) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.subRemoteSet {
		r.subPending = append(r.subPending, cand)
		return
	}
	r.subPC.AddICECandidate(cand)
}

func (r *SFURelay) CreatePubRenegotiate() (webrtc.SessionDescription, error) {
	offer, err := r.pubPC.CreateOffer(&webrtc.OfferOptions{ICERestart: false})
	if err != nil {
		return offer, err
	}
	err = r.pubPC.SetLocalDescription(offer)
	if err != nil {
		return offer, err
	}
	offer.SDP = tmapi.MungeSDPAddVideoContent(offer.SDP)
	r.mu.Lock()
	r.pubRemoteSet = false
	r.pubPending = nil
	r.mu.Unlock()
	return offer, nil
}

func (r *SFURelay) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	mt, tun, layer := r.mt, r.tun, r.delivered
	r.mu.Unlock()
	if k, ok := layer.(*rtc.MultiTrackKCPTunnel); ok {
		k.StopLayer()
	}
	if mt != nil {
		mt.Stop()
	}
	if tun != nil {
		tun.Stop()
	}
	if r.pubPC != nil {
		r.pubPC.Close()
	}
	if r.subPC != nil {
		r.subPC.Close()
	}
}

func (r *SFURelay) readTrack(track *webrtc.TrackRemote) {
	if track.Codec().MimeType != webrtc.MimeTypeVP8 {
		buf := make([]byte, common.UDPBufSize)
		for {
			if _, _, err := track.Read(buf); err != nil {
				return
			}
		}
	}

	var vp8Pkt codecs.VP8Packet
	var frameBuf []byte
	var lastSeq uint16
	var haveLastSeq bool
	frameValid := false
	var recvCount int
	bufSz := r.readBufSize
	if bufSz <= 0 {
		bufSz = common.RTPBufSize
	}
	buf := make([]byte, bufSz)
	for {
		n, _, err := track.Read(buf)
		if err != nil {
			return
		}
		pkt := &rtp.Packet{}
		if pkt.Unmarshal(buf[:n]) != nil {
			continue
		}
		if haveLastSeq && pkt.SequenceNumber != lastSeq+1 {
			frameValid = false
			frameBuf = frameBuf[:0]
		}
		lastSeq = pkt.SequenceNumber
		haveLastSeq = true

		vp8Payload, err := vp8Pkt.Unmarshal(pkt.Payload)
		if err != nil {
			frameValid = false
			frameBuf = frameBuf[:0]
			continue
		}
		if vp8Pkt.S == 1 {
			frameBuf = frameBuf[:0]
			frameValid = true
		}
		if !frameValid {
			continue
		}
		frameBuf = append(frameBuf, vp8Payload...)
		if !pkt.Marker {
			continue
		}
		recvCount++
		if common.Debug && (recvCount <= 3 || recvCount%200 == 0) {
			log.Printf("[video] recv vp8 frame #%d %d bytes", recvCount, len(frameBuf))
		}

		res := r.obf.Decode(frameBuf)
		frameBuf = frameBuf[:0]
		frameValid = false

		if !res.HasFrame || res.SelfEcho {
			continue
		}
		if res.PeerRestart {
			log.Printf("[video] peer restart detected, new epoch=0x%08x", res.PeerEpoch)
			r.resetForNewPeer()
			if r.OnPeerRestart != nil {
				r.OnPeerRestart()
			}
		}
		if res.Keepalive || len(res.Payload) == 0 {
			continue
		}
		r.mu.Lock()
		tun := r.tun
		r.mu.Unlock()
		if tun != nil && tun.OnData != nil {
			tun.OnData(res.Payload)
		}
	}
}
