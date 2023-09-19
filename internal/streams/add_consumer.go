package streams

import (
	"errors"
	"strings"
	"sync/atomic"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func (s *Stream) AddConsumer(cons core.Consumer) (err error) {
	// support for multiple simultaneous requests from different consumers
	consN := atomic.AddInt32(&s.requests, 1) - 1

	var prodErrors []error
	var prodMedias []*core.Media
	var prods []*Producer // matched producers for consumer

	// Step 1. Get consumer medias
	consMedias := cons.GetMedias()
	for _, consMedia := range consMedias {
		log.Trace().Msgf("[streams] check cons=%d media=%s", consN, consMedia)

	producers:
		for prodN, prod := range s.producers {
			if err = prod.Dial(); err != nil {
				log.Trace().Err(err).Msgf("[streams] skip prod=%s", prod.url)
				prodErrors = append(prodErrors, err)
				continue
			}

			// Step 2. Get producer medias (not tracks yet)
			for _, prodMedia := range prod.GetMedias() {
				log.Trace().Msgf("[streams] check prod=%d media=%s", prodN, prodMedia)
				prodMedias = append(prodMedias, prodMedia)

				// Step 3. Match consumer/producer codecs list
				prodCodec, consCodec := prodMedia.MatchMedia(consMedia)
				if prodCodec == nil {
					continue
				}

				var track *core.Receiver

				switch prodMedia.Direction {
				case core.DirectionRecvonly:
					log.Trace().Msgf("[streams] match prod=%d => cons=%d", prodN, consN)

					// Step 4. Get recvonly track from producer
					if track, err = prod.GetTrack(prodMedia, prodCodec); err != nil {
						log.Info().Err(err).Msg("[streams] can't get track")
						continue
					}
					// Step 5. Add track to consumer
					if err = cons.AddTrack(consMedia, consCodec, track); err != nil {
						log.Info().Err(err).Msg("[streams] can't add track")
						continue
					}

				case core.DirectionSendonly:
					log.Trace().Msgf("[streams] match cons=%d => prod=%d", consN, prodN)

					// Step 4. Get recvonly track from consumer (backchannel)
					if track, err = cons.(core.Producer).GetTrack(consMedia, consCodec); err != nil {
						log.Info().Err(err).Msg("[streams] can't get track")
						continue
					}
					// Step 5. Add track to producer
					if err = prod.AddTrack(prodMedia, prodCodec, track); err != nil {
						log.Info().Err(err).Msg("[streams] can't add track")
						continue
					}
				}

				prods = append(prods, prod)

				if !consMedia.MatchAll() {
					break producers
				}
			}
		}
	}

	// stop producers if they don't have readers
	if atomic.AddInt32(&s.requests, -1) == 0 {
		s.stopProducers()
	}

	if len(prods) == 0 {
		return formatError(consMedias, prodMedias, prodErrors)
	}

	s.mu.Lock()
	s.consumers = append(s.consumers, cons)
	s.mu.Unlock()

	// there may be duplicates, but that's not a problem
	for _, prod := range prods {
		prod.start()
	}

	return nil
}

func formatError(consMedias, prodMedias []*core.Media, prodErrors []error) error {
	if prodMedias != nil {
		var prod, cons string

		for _, media := range prodMedias {
			if media.Direction == core.DirectionRecvonly {
				for _, codec := range media.Codecs {
					prod = appendString(prod, codec.PrintName())
				}
			}
		}

		for _, media := range consMedias {
			if media.Direction == core.DirectionSendonly {
				for _, codec := range media.Codecs {
					cons = appendString(cons, codec.PrintName())
				}
			}
		}

		return errors.New("streams: codecs not matched: " + prod + " => " + cons)
	}

	if prodErrors != nil {
		var text string

		for _, err := range prodErrors {
			text = appendString(text, err.Error())
		}

		return errors.New("streams: " + text)
	}

	return errors.New("streams: unknown error")
}

func appendString(s, elem string) string {
	if strings.Contains(s, elem) {
		return s
	}
	if len(s) == 0 {
		return elem
	}
	return s + ", " + elem
}
