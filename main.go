package main

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
	"github.com/xjezzz/discord-recording-bot/config"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	err := config.ReadConfig()

	dg, err := discordgo.New("Bot " + config.Token)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	dg.AddHandler(channelCreate)
	dg.AddHandler(voiceStateUpdate)

	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	err = dg.Close()
	if err != nil {
		if err.Error() == "websocket: close sent" {
			fmt.Println("Bot closed connection")
		}
	}
}

func channelCreate(s *discordgo.Session, event *discordgo.ChannelCreate) {
	if event.GuildID == "" || event.Channel.Type != discordgo.ChannelTypeGuildVoice {
		return
	}

	// Присоединяемся к голосовому каналу
	v, err := s.ChannelVoiceJoin(event.GuildID, event.Channel.ID, false, false)
	if err != nil {
		fmt.Println("failed to join voice channel:", err)
		return
	}
	if !v.Ready {
		fmt.Println("Voice connection is not ready")
		return

	}

	voiceHandler(v.OpusRecv, event.GuildID, event.Channel.ID)
}

func voiceHandler(c chan *discordgo.Packet, guildID, channelID string) {

	file, err := oggwriter.New(fmt.Sprintf("%s_%s.ogg", guildID, channelID), 48000, 2)
	if err != nil {
		fmt.Printf("failed to create file %s.ogg, giving up on recording: %v\n", guildID, err)
		return
	}
	defer file.Close()

	for vp := range c {
		rtp := createPionRTPPacket(vp)
		err := file.WriteRTP(rtp)
		if err != nil {
			fmt.Printf("failed to write to file %s_%d.ogg, giving up on recording: %v\n", guildID, vp.SSRC, err)
		}

	}
}

func voiceStateUpdate(s *discordgo.Session, event *discordgo.VoiceStateUpdate) {
	// Проверяем, что BeforeUpdate не равен nil
	if event.BeforeUpdate != nil && event.BeforeUpdate.ChannelID != "" {
		guild, err := s.State.Guild(event.GuildID)
		if err != nil {
			fmt.Println("failed to get voice channel:", err)
			return
		}
		// Проверяем количество пользователей в голосовом канале
		userCount := 0
		botInSameChannel := false
		for _, vs := range guild.VoiceStates {
			if vs.ChannelID == event.BeforeUpdate.ChannelID && vs.UserID != s.State.User.ID {
				userCount++
			}
			if vs.ChannelID == event.BeforeUpdate.ChannelID && vs.UserID == s.State.User.ID {
				fmt.Println("herek")
				botInSameChannel = true
			}
		}
		fmt.Println("COUNTER", userCount)
		if userCount == 0 && botInSameChannel {
			_, err := s.ChannelVoiceJoin(event.GuildID, "", false, false)
			if err != nil {
				fmt.Println("failed to disconnect from voice channel:", err)
			}
			fmt.Println("BOT VISHEL")
		}
	}
}

func createPionRTPPacket(p *discordgo.Packet) *rtp.Packet {
	return &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    0x78, // Taken from Discord voice docs
			SequenceNumber: p.Sequence,
			Timestamp:      p.Timestamp,
			SSRC:           p.SSRC,
		},
		Payload: p.Opus,
	}
}
