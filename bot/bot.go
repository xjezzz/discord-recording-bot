package bot

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/pion/rtp"
	"os"
	"sync"
	"time"
)

func createPionRTPPacket(p *discordgo.Packet) *rtp.Packet {
	payload := make([]byte, len(p.Opus))
	copy(payload, p.Opus)
	return &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    120, // Discord voice docs suggest using 120 for Opus
			SequenceNumber: uint16(p.Sequence),
			Timestamp:      p.Timestamp,
			SSRC:           p.SSRC,
		},
		Payload: payload,
	}
}

func VoiceCreate(s *discordgo.Session, vs *discordgo.VoiceStateUpdate, c chan *discordgo.Packet) {
	v, err := s.ChannelVoiceJoin(vs.GuildID, vs.ChannelID, false, true)
	if err != nil {
		fmt.Println("failed to join voice channel:", err)
		v.Disconnect()
		return
	}

	defer v.Close()

	files := make(map[uint32]*os.File)
	var usersInChannel int
	var mutex sync.Mutex

	// Отложенная функция для проверки и выхода из канала при пустоте
	defer func() {
		for {
			mutex.Lock()
			if usersInChannel == 0 {
				v.Disconnect()
				break
			}
			mutex.Unlock()
			time.Sleep(1 * time.Second) // Проверяем каждую секунду
		}
	}()

	// Горутина для записи аудио
	go func() {
		for p := range c {
			mutex.Lock()
			usersInChannel++ // Увеличиваем количество пользователей в канале
			mutex.Unlock()

			file, ok := files[p.SSRC]
			if !ok {
				mutex.Lock()
				file, err = os.Create(fmt.Sprintf("%d.ogg", p.SSRC))
				if err != nil {
					fmt.Printf("failed to create file %d.ogg, giving up on recording: %v\n", p.SSRC, err)
					mutex.Unlock()
					return
				}
				files[p.SSRC] = file
				mutex.Unlock()
			}
			// Construct pion RTP packet from DiscordGo's type.
			rtp := createPionRTPPacket(p)
			_, err := file.Write(rtp.Payload)
			if err != nil {
				fmt.Printf("failed to write to file %d.ogg, giving up on recording: %v\n", p.SSRC, err)
			}

			mutex.Lock()
			usersInChannel-- // Уменьшаем количество пользователей в канале
			mutex.Unlock()
		}
	}()
}
