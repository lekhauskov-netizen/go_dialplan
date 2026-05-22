package main

import (
        "bufio"
        "fmt"
        "log"
        "net"
        "strings"
)

const (
        port        = ":8494"
        maxRetries  = 3
)

func main() {
        listener, err := net.Listen("tcp", port)
        if err != nil {
                log.Fatal(err)
        }
        log.Printf("FastAGI сервер запущен на %s", port)

        for {
                conn, err := listener.Accept()
                if err != nil {
                        log.Print(err)
                        continue
                }
                go handle(conn)
        }
}

func handle(conn net.Conn) {
        defer conn.Close()

        agi := NewAGI(conn)
        agi.ReadEnv()

        callerID := agi.GetVar("caller_id")
        channel := agi.GetVar("channel")

        log.Printf("Новый звонок: callerID=%s, channel=%s", callerID, channel)

        retries := 0

        for {
                // Воспроизводим приветствие и ждём ввод
                resp := agi.Exec("GET DATA", "ru/custom/ivr-welcome 5 1")

                digit := ""
                if strings.HasPrefix(resp, "200") {
                        parts := strings.Split(resp, "=")
                        if len(parts) == 2 {
                                digit = strings.TrimSpace(parts[1])
                        }
                }

                log.Printf("Получена цифра: '%s' (resp: %s)", digit, resp)

                switch digit {
                case "1":
                        log.Printf("Маршрутизация на 101")
                        agi.Exec("EXEC Dial", "PJSIP/101,30")
                        agi.Exec("HANGUP", "")
                        return
                case "2":
                        log.Printf("Маршрутизация на 102")
                        agi.Exec("EXEC Dial", "PJSIP/102,30")
                        agi.Exec("HANGUP", "")
                        return
                case "", "-1":
                        retries++
                        log.Printf("Таймаут, попытка %d из %d", retries, maxRetries)
                        agi.Exec("STREAM FILE", "ru/custom/ivr-invalid \"\"")

                        if retries >= maxRetries {
                                log.Printf("Превышено число попыток")
                                agi.Exec("HANGUP", "")
                                return
                        }
                default:
                        retries++
                        log.Printf("Неверная цифра '%s', попытка %d из %d", digit, retries, maxRetries)
                        agi.Exec("STREAM FILE", "ru/custom/ivr-invalid \"\"")

                        if retries >= maxRetries {
                                log.Printf("Превышено число попыток")
                                agi.Exec("HANGUP", "")
                                return
                        }
                }
        }
}

type AGI struct {
        conn net.Conn
        env  map[string]string
}

func NewAGI(conn net.Conn) *AGI {
        return &AGI{conn: conn, env: make(map[string]string)}
}

func (a *AGI) ReadEnv() {
        reader := bufio.NewReader(a.conn)
        for {
                line, err := reader.ReadString('\n')
                if err != nil {
                        return
                }
                line = strings.TrimSpace(line)
                if line == "" {
                        break
                }
                parts := strings.SplitN(line, ":", 2)
                if len(parts) == 2 {
                        key := strings.TrimSpace(parts[0])
                        value := strings.TrimSpace(parts[1])
                        if strings.HasPrefix(key, "agi_") {
                                key = strings.TrimPrefix(key, "agi_")
                        }
                        a.env[key] = value
                }
        }
}

func (a *AGI) GetVar(name string) string {
        return a.env[name]
}

func (a *AGI) Exec(command, args string) string {
        cmd := fmt.Sprintf("%s %s\n", command, args)
        a.conn.Write([]byte(cmd))

        reader := bufio.NewReader(a.conn)
        resp, err := reader.ReadString('\n')
        if err != nil {
                log.Printf("Ошибка выполнения %s: %v", command, err)
                return ""
        }
        return strings.TrimSpace(resp)
}
