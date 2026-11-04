package main

import (
	"log"
	"os"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	a := newApp()
	for _, arg := range os.Args[1:] {
		if !strings.HasPrefix(arg, "-") {
			a.initialPath = arg
			break
		}
	}

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "微信小程序解包工具",
			Width:     1180,
			Height:    760,
			MinWidth:  900,
			MinHeight: 560,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})
		a.start(win)
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
