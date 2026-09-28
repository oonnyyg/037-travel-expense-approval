package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/travel-expense-approval/internal/desk"
)

func main() {
	stateDir := flag.String("state", "", "数据目录（持久化单据与操作历史）")
	flag.Parse()
	if *stateDir == "" {
		fmt.Fprintln(os.Stderr, "必须通过 --state 指定数据目录")
		os.Exit(2)
	}

	d, err := desk.Load(*stateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载状态失败: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req desk.Request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = enc.Encode(desk.Response{OK: false, Error: "invalid_json", Message: "请求不是合法 JSON: " + err.Error()})
			continue
		}
		_ = enc.Encode(d.Handle(req))
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "读取输入失败: %v\n", err)
		os.Exit(1)
	}
}
