package streams

import (
	"errors"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func (s *Stream) AddConsumer(cons core.Consumer) (err error) {
	// support for multiple simultaneous pending from different consumers
	consN := s.pending.Add(1) - 1

	var prodErrors = make([]error, len(s.producers))
	var prodMedias []*core.Media
	var prodStarts []*Producer

	// Step 1. Get consumer medias
	consMedias := cons.GetMedias()
	for _, consMedia := range consMedias {
		log.Trace().Msgf("[streams] check cons=%d media=%s", consN, consMedia)

	producers:
		for prodN, prod := range s.producers {
			if prodErrors[prodN] != nil {
				log.Trace().Msgf("[streams] skip cons=%d prod=%d", consN, prodN)
				continue
			}

			if err = prod.Dial(); err != nil {
				log.Trace().Err(err).Msgf("[streams] dial cons=%d prod=%d", consN, prodN)
				prodErrors[prodN] = err
				continue
			}

			// Step 2. Get producer medias (not tracks yet)
			for _, prodMedia := range prod.GetMedias() {
				log.Trace().Msgf("[streams] check cons=%d prod=%d media=%s", consN, prodN, prodMedia)
				prodMedias = append(prodMedias, prodMedia)

				// Step 3. Match consumer/producer codecs list
				prodCodec, consCodec := prodMedia.MatchMedia(consMedia)
				if prodCodec == nil {
					continue
				}

				var track *core.Receiver

				switch prodMedia.Direction {
				case core.DirectionRecvonly:
					log.Trace().Msgf("[streams] match cons=%d <= prod=%d", consN, prodN)

					// Step 4. Get recvonly track from producer
					if track, err = prod.GetTrack(prodMedia, prodCodec); err != nil {
						log.Info().Err(err).Msg("[streams] can't get track")
						prodErrors[prodN] = err
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
						prodErrors[prodN] = err
						continue
					}
				}

				prodStarts = append(prodStarts, prod)

				if !consMedia.MatchAll() {
					break producers
				}
			}
		}
	}

	// stop producers if they don't have readers
	if s.pending.Add(-1) == 0 {
		s.stopProducers()
	}

	if len(prodStarts) == 0 {
		return formatError(consMedias, prodMedias, prodErrors)
	}

	s.mu.Lock()
	s.consumers = append(s.consumers, cons)
	s.mu.Unlock()

	// there may be duplicates, but that's not a problem
	for _, prod := range prodStarts {
		prod.start()
	}

	return nil
}

func formatError(consMedias, prodMedias []*core.Media, prodErrors []error) error {
	// 1. Return errors if any not nil
	var text string

	for _, err := range prodErrors {
		if err != nil {
			text = appendString(text, err.Error())
		}
	}

	if len(text) != 0 {
		return errors.New("streams: " + text)
	}

	// 2. Return "codecs not matched"
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

	// 3. Return unknown error
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
