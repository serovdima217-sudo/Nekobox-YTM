// Linux-замена Android VpnService: привилегирован только процесс сетевого ядра.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Режим только чтения /proc, без сигналов, сетевых изменений и запуска ядра.
func listProcesses() error {
	type row struct {
		PID  int    `json:"pid"`
		Name string `json:"name"`
		Path string `json:"path"`
		UID  int    `json:"uid"`
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return err
	}
	rows := make([]row, 0, len(entries))
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		base := filepath.Join("/proc", entry.Name())
		comm, err := os.ReadFile(filepath.Join(base, "comm"))
		if err != nil {
			continue
		}
		exe, _ := os.Readlink(filepath.Join(base, "exe"))
		status, _ := os.ReadFile(filepath.Join(base, "status"))
		uid := -1
		for _, line := range strings.Split(string(status), "\n") {
			if strings.HasPrefix(line, "Uid:") {
				fields := strings.Fields(line)
				if len(fields) > 1 {
					uid, _ = strconv.Atoi(fields[1])
				}
				break
			}
		}
		rows = append(rows, row{pid, strings.TrimSpace(string(comm)), strings.TrimSuffix(exe, " (deleted)"), uid})
	}
	return json.NewEncoder(os.Stdout).Encode(rows)
}

func main() {
	listOnly := flag.Bool("list-processes", false, "Прочитать список процессов, не изменяя VPN")
	core := flag.String("core", "", "Ядро")
	config := flag.String("config", "", "Конфигурация ядра")
	carrier := flag.String("carrier", "", "Клиент SplitYTM")
	carrierConfig := flag.String("carrier-config", "", "Конфигурация SplitYTM")
	parent := flag.Int("parent", 0, "Процесс интерфейса")
	plugins := flag.String("plugins", "", "План внешних процессов Linux")
	flag.Parse()
	if *listOnly {
		if err := listProcesses(); err != nil {
			fmt.Fprintln(os.Stderr, "Не удалось прочитать процессы:", err)
			os.Exit(1)
		}
		return
	}
	if *core == "" || *config == "" || *parent <= 1 {
		fmt.Fprintln(os.Stderr, "Неполные параметры службы Linux")
		os.Exit(2)
	}
	parentState := func() string {
		b, e := os.ReadFile(fmt.Sprintf("/proc/%d/stat", *parent))
		if e != nil {
			return ""
		}
		end := strings.LastIndexByte(string(b), ')')
		fields := strings.Fields(string(b)[end+1:])
		if len(fields) < 20 {
			return ""
		}
		return fields[19]
	}
	initial := parentState()
	if initial == "" {
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if parentState() != initial {
					cancel()
					return
				}
			}
		}
	}()
	type child struct {
		cmd  *exec.Cmd
		done chan error
	}
	var children []child
	start := func(path string, extra map[string]string, args ...string) error {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("Требуется абсолютный путь")
		}
		cmd := exec.Command(path, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(), "ENABLE_DEPRECATED_TUN_ADDRESS_X=true", "ENABLE_DEPRECATED_SPECIAL_OUTBOUNDS=true", "ENABLE_DEPRECATED_LEGACY_DNS_SERVERS=true", "HOME="+filepath.Dir(*config), "XDG_CACHE_HOME="+filepath.Dir(*config))
		for key, value := range extra {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		c := child{cmd, make(chan error, 1)}
		children = append(children, c)
		go func() { err := cmd.Wait(); c.done <- err; cancel() }()
		fmt.Fprintln(os.Stdout, "Запущена служба Linux, PID", strconv.Itoa(cmd.Process.Pid))
		return nil
	}
	if *carrier != "" {
		if err := start(*carrier, nil, "-config", *carrierConfig); err != nil {
			fmt.Fprintln(os.Stderr, "Не запущен SplitYTM:", err)
			os.Exit(1)
		}
	}
	// Все внешние процессы принадлежат только этому экземпляру и очищаются вместе с ним.
	if *plugins != "" && ctx.Err() == nil {
		type plugin struct {
			Path string            `json:"path"`
			Args []string          `json:"args"`
			Env  map[string]string `json:"env"`
		}
		var plan []plugin
		data, err := os.ReadFile(*plugins)
		if err != nil || len(data) > 16*1024*1024 || json.Unmarshal(data, &plan) != nil || len(plan) > 256 {
			fmt.Fprintln(os.Stderr, "Не удалось прочитать план Linux-плагинов")
			cancel()
		} else {
			for _, entry := range plan {
				if ctx.Err() != nil {
					break
				}
				if err := start(entry.Path, entry.Env, entry.Args...); err != nil {
					fmt.Fprintln(os.Stderr, "Не запущен Linux-плагин:", err)
					cancel()
					break
				}
			}
		}
	}
	var coreProcess *os.Process
	if ctx.Err() == nil {
		if err := start(*core, nil, "run", "-c", *config); err != nil {
			fmt.Fprintln(os.Stderr, "Не запущено ядро:", err)
			cancel()
		} else {
			coreProcess = children[len(children)-1].cmd.Process
		}
	}
	// Команды поступают только от создавшего службу интерфейса, не из сети.

	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			command := strings.TrimSpace(scanner.Text())
			if command == "Перезагрузить" {
				if coreProcess != nil && ctx.Err() == nil {
					if err := coreProcess.Signal(syscall.SIGHUP); err != nil {
						fmt.Fprintln(os.Stderr, "Не удалось обновить маршруты Linux:", err)
					} else {
						fmt.Fprintln(os.Stdout, "Обновление маршрутов Linux: клиент SplitYTM остаётся подключённым")
					}
				}
			} else {
				cancel()
				return
			}
		}
		cancel()
	}()
	<-ctx.Done()
	// Сначала ядро возвращает маршруты, затем клиент освобождает серверную пару.
	for i := len(children) - 1; i >= 0; i-- {
		c := children[i]
		_ = c.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-c.done:
		case <-time.After(5 * time.Second):
			_ = c.cmd.Process.Kill()
			<-c.done
		}
	}
	fmt.Fprintln(os.Stdout, "Служба Linux остановлена, маршруты и пара освобождены")
}
