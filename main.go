package main

import (
	"flag"
)

func main() {
	listFlag := flag.Bool("list", false, "List all contacts in fzf picker")
	jidFlag := flag.String("jid", "", "Directly specify JID for testing")
	flag.Parse()

	run(*listFlag, *jidFlag)
}
