package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"text/template"

	"github.com/spf13/pflag"
	"github.com/togettoyou/hub-mirror/pkg"
)

var (
	content    = pflag.StringP("content", "", "", "原始镜像，格式为：直接输入镜像名称，多个镜像用逗号分隔；或 JSON 格式：{ \"platform\": \"\", \"hub-mirror\": [] }")
	maxContent = pflag.IntP("maxContent", "", 11, "原始镜像个数限制")
	repository = pflag.StringP("repository", "", "", "推送仓库地址，为空默认为 hub.docker.com")
	username   = pflag.StringP("username", "", "", "仓库用户名")
	password   = pflag.StringP("password", "", "", "仓库密码")
	outputPath = pflag.StringP("outputPath", "", "output.md", "结果输出路径")
)

func parseInput(input string) (string, []string, error) {
	// 尝试解析为 JSON 格式
	var hubMirrors struct {
		HubMirror []string `json:"hub-mirror"`
		Platform  string   `json:"platform"`
	}
	if json.Unmarshal([]byte(input), &hubMirrors) == nil {
		return hubMirrors.Platform, hubMirrors.HubMirror, nil
	}

	// 如果不是 JSON，则解析为逗号分隔的镜像列表
	mirrors := strings.Split(input, ",")
	for i := range mirrors {
		mirrors[i] = strings.TrimSpace(mirrors[i])
	}
	if len(mirrors) == 0 {
		return "", nil, errors.New("输入不能为空")
	}
	return "", mirrors, nil
}

func main() {
	pflag.Parse()

	fmt.Println("验证原始镜像内容")
	platform, mirrors, err := parseInput(*content)
	if err != nil {
		panic(err)
	}

	if len(mirrors) > *maxContent {
		panic("提交的原始镜像个数超出了最大限制")
	}

	fmt.Printf("mirrors: %+v, platform: %+v\n", mirrors, platform)

	fmt.Println("初始化 Docker 客户端")
	cli, err := pkg.NewCli(context.Background(), *repository, *username, *password, os.Stdout)
	if err != nil {
		panic(err)
	}

	outputs := make([]*pkg.Output, 0)
	mu := sync.Mutex{}
	wg := sync.WaitGroup{}

	for _, source := range mirrors {
		source := source

		if source == "" {
			continue
		}
		fmt.Println("开始转换镜像", source)
		wg.Add(1)
		go func() {
			defer wg.Done()

			output, err := cli.PullTagPushImage(context.Background(), source, platform)
			if err != nil {
				fmt.Println(source, "转换异常: ", err)
				return
			}

			mu.Lock()
			defer mu.Unlock()
			outputs = append(outputs, output)
		}()
	}

	wg.Wait()

	if len(outputs) == 0 {
		panic("没有转换成功的镜像")
	}

	tmpl, err := template.ParseFiles("output.tmpl")
	if err != nil {
		panic(err)
	}
	outputFile, err := os.Create(*outputPath)
	if err != nil {
		panic(err)
	}
	defer outputFile.Close()

	err = tmpl.Execute(outputFile, map[string]interface{}{
		"Outputs":    outputs,
		"Repository": *repository,
	})
	if err != nil {
		panic(err)
	}
}
