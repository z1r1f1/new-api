//go:build ignore

package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

//go:embed web/classic/dist
var classicFS embed.FS

func main() {
	entries, err := fs.ReadDir(classicFS, "web/classic/dist/assets")
	if err != nil {
		fmt.Println("Error reading embedded FS:", err)
		os.Exit(1)
	}
	for _, e := range entries {
		info, _ := e.Info()
		if strings.HasPrefix(e.Name(), "index-") && strings.HasSuffix(e.Name(), ".js") && info.Size() > 7000000 {
			data, err := fs.ReadFile(classicFS, "web/classic/dist/assets/"+e.Name())
			if err != nil {
				fmt.Println("Error reading:", err)
				continue
			}
			fmt.Printf("File: %s, Size: %d\n", e.Name(), len(data))
			if strings.Contains(string(data), "request_headers") {
				fmt.Println(">> FOUND request_headers!")
			} else {
				fmt.Println(">> NOT FOUND request_headers")
			}
		}
	}
}
