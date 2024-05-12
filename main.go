package main

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3/pkg/media"
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

	dg.Close()
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

	voiceHandler(s, v.OpusRecv, event.GuildID)
}

func voiceHandler(s *discordgo.Session, c chan *discordgo.Packet, guildID string) {
	files := make(map[uint32]media.Writer)
	userWasInChannel := false

	for vp := range c {
		file, ok := files[vp.SSRC]
		if !ok {
			var err error
			file, err = oggwriter.New(fmt.Sprintf("%s_%d.ogg", guildID, vp.SSRC), 48000, 2)
			if err != nil {
				fmt.Printf("failed to create file %s_%d.ogg, giving up on recording: %v\n", guildID, vp.SSRC, err)
				return
			}
			files[vp.SSRC] = file
		}

		rtp := createPionRTPPacket(vp)
		err := file.WriteRTP(rtp)
		if err != nil {
			fmt.Printf("failed to write to file %s_%d.ogg, giving up on recording: %v\n", guildID, vp.SSRC, err)
		}

		// Check if any users are in the voice channel
		guild, _ := s.State.Guild(guildID)
		userCount := 0
		for _, vs := range guild.VoiceStates {
			if vs.UserID != s.State.User.ID {
				userCount++
				userWasInChannel = true
			}
		}

		// If no users are in the channel and a user was previously in the channel, disconnect
		if userCount == 0 && userWasInChannel {
			fmt.Println("All members left, disconnecting from voice channel")
			vc, _ := s.ChannelVoiceJoin(guildID, "", false, false)
			vc.Disconnect()
			return
		}
	}

	for _, f := range files {
		f.Close()
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
		for _, vs := range guild.VoiceStates {
			if vs.ChannelID == event.BeforeUpdate.ChannelID && vs.UserID != s.State.User.ID {
				userCount++
			}
		}
		fmt.Println("COUNTER", userCount)
		if userCount == 0 {
			_, err := s.ChannelVoiceJoin(event.GuildID, "", false, false)
			fmt.Println("BOT VISHEL")
			if err != nil {
				fmt.Println("failed to disconnect from voice channel:", err)
			}
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
