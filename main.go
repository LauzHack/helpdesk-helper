package main

import (
	"fmt"
	"lauzhack-bot/config"
	"lauzhack-bot/discord"
	"lauzhack-bot/scheduler"
	"lauzhack-bot/server"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

func main() {
	// Config + store
	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <config.json>", os.Args[0])
	}
	if err := config.LoadConfig(os.Args[1]); err != nil {
		log.Fatalf("config: %v", err)
	}
	var err error
	config.Loc, err = time.LoadLocation(config.Cfg.Timezone)
	if err != nil {
		log.Fatalf("timezone: %v", err)
	}
	if err := config.LoadStore(); err != nil {
		log.Fatalf("store: %v", err)
	}

	server.Start(config.Cfg.DataFile, ":8080")

	// Discord session
	dg, err := discordgo.New("Bot " + config.Cfg.Token)
	if err != nil {
		log.Fatalf("discord: %v", err)
	}

	dg.Identify.Intents = discordgo.IntentsAll
	dg.AddHandler(discord.OnInteractionCreate)

	if err := dg.Open(); err != nil {
		log.Fatalf("open ws: %v", err)
	}
	defer func() {
		if err := dg.Close(); err != nil {
			log.Printf("error closing discord session: %v", err)
		}
	}()

	_ = dg.UpdateGameStatus(0, "Managing helpdesk shifts")

	// Command registration
	if err := registerCommands(dg, config.Cfg.GuildID); err != nil {
		log.Printf("command registration: %v", err)
	}

	bot := &discord.DiscordBot{Session: dg}
	scheduler.Init(bot)
	go scheduler.SchedulerLoop()

	// Wait
	log.Printf("running. guild=%s role=%s tz=%s", config.Cfg.GuildID, config.Cfg.RoleID, config.Cfg.Timezone)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down")
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
