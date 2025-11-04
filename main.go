package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
)

var token string

func init() {
	flag.StringVar(&token, "t", "", "Discord bot token")
	flag.Parse()

	if token == "" {
		log.Fatalf("Missing required -t <token> argument.")
	}
}

func main() {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("error creating discord session: %v", err)
	}
	defer session.Close()

	session.AddHandler(onMessage)
	session.Identify.Intents = discordgo.IntentsAll

	if err := session.Open(); err != nil {
		log.Fatalf("error opening connection: %v", err)
	}

	fmt.Println("Bot running. Press CTRL-C to exit.")
	waitForExit()
}

func waitForExit() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-stop
}

func onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	_, err := s.ChannelMessageSend(m.ChannelID, "<@&888855737500049468>")
	if err != nil {
		log.Fatalf("unable to send a message: %v", err)
	}

	switch m.Content {
	case "ping":
		m, err := s.ChannelMessageSend(m.ChannelID, m.Author.Mention()+"Pong!")
		if err != nil {
			log.Fatalf("unable to send a message: %v", err)
		}
		println(m.Content)

	case "pong":
		s.ChannelMessageSend(m.ChannelID, "Ping!")
	}
}
