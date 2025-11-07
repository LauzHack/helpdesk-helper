package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lauzhack-bot/config"
	"lauzhack-bot/discord"
	"lauzhack-bot/scheduler"
	"lauzhack-bot/server"
	"lauzhack-bot/store"

	"github.com/bwmarrin/discordgo"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <config.json>", os.Args[0])
	}

	if err := config.LoadConfig(os.Args[1]); err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := store.Init(config.Cfg.DataFile); err != nil {
		log.Fatalf("store: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverAddr := ":8080"
	go server.Start(ctx, serverAddr)

	dg, err := discordgo.New("Bot " + config.Cfg.Token)
	if err != nil {
		log.Fatalf("discord: %v", err)
	}

	dg.Identify.Intents = discordgo.IntentsAll
	dg.AddHandler(discord.OnInteractionCreate)

	if err := dg.Open(); err != nil {
		log.Fatalf("discord open: %v", err)
	}
	defer func() {
		if err := dg.Close(); err != nil {
			log.Printf("error closing discord session: %v", err)
		}
	}()

	if err := registerCommands(dg, config.Cfg.GuildID); err != nil {
		log.Printf("command registration failed: %v", err)
	}

	bot := discord.New(dg)
	discord.Init(bot)
	scheduler.Init(bot)
	server.Init(dg)

	go scheduler.SchedulerLoop()

	log.Printf("helpdesk-helper running | guild=%s | role=%s | server=%s",
		config.Cfg.GuildID, config.Cfg.RoleID, serverAddr)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutdown signal received")
	cancel()

	// Allow in-flight operations to finish
	time.Sleep(1 * time.Second)

	log.Println("shutdown complete")
}

func registerCommands(dg *discordgo.Session, guildID string) error {
	cmds := []*discordgo.ApplicationCommand{
		{
			Name:        "helpdesk",
			Description: "User helpdesk actions",
			Options: []*discordgo.ApplicationCommandOption{
				{Name: "join", Description: "Join helpdesk", Type: discordgo.ApplicationCommandOptionSubCommand},
				{Name: "leave", Description: "Leave helpdesk", Type: discordgo.ApplicationCommandOptionSubCommand},
				{Name: "list", Description: "Show current and next shifts", Type: discordgo.ApplicationCommandOptionSubCommand},
			},
		},
		{
			Name:        "admin",
			Description: "Administrative commands",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "helpdesk",
					Description: "Administer helpdesk operations",
					Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
					Options: []*discordgo.ApplicationCommandOption{
						makeUserCmd("add", "Force add user to helpdesk role now"),
						makeUserCmd("remove", "Force remove user from helpdesk role now"),
						{Name: "sync", Description: "Reconcile helpdesk role with schedule (manual)", Type: discordgo.ApplicationCommandOptionSubCommand},
					},
				},
			},
		},
	}

	for _, c := range cmds {
		created, err := dg.ApplicationCommandCreate(dg.State.User.ID, guildID, c)
		if err != nil {
			return fmt.Errorf("register %s: %w", c.Name, err)
		}
		log.Printf("registered /%s (id=%s)", created.Name, created.ID)
	}
	return nil
}

func makeUserCmd(name, desc string) *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Name:        name,
		Description: desc,
		Type:        discordgo.ApplicationCommandOptionSubCommand,
		Options: []*discordgo.ApplicationCommandOption{
			{Name: "user", Description: "Target user", Type: discordgo.ApplicationCommandOptionUser, Required: true},
		},
	}
}
