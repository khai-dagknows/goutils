package conc

import (
	"errors"
	"time"
)

type ReaderFunc[R any] func() (R, error)

type ReaderChan[R any] struct {
	ReaderWriterBase[R]
	msgChannel chan ValueOrError[R]
	Read       ReaderFunc[R]
	OnClose    func()
}

func NewReader[R any](read ReaderFunc[R]) *ReaderChan[R] {
	out := ReaderChan[R]{
		ReaderWriterBase: ReaderWriterBase[R]{
			WaitTime: 10 * time.Second,
		},
		Read: read,
	}
	go out.start()
	return &out
}

func (ch *ReaderChan[T]) cleanup() {
	close(ch.msgChannel)
	ch.msgChannel = nil
	ch.ReaderWriterBase.cleanup()
}

/**
 * Returns whether the connection reader/writer loops are running.
 */
func (ch *ReaderChan[R]) IsRunning() bool {
	return ch.msgChannel != nil
}

/**
 * Returns the conn's reader channel.
 */
func (rc *ReaderChan[R]) ResultChannel() chan ValueOrError[R] {
	return rc.msgChannel
}

/**
 * This method is called to stop the channel
 * If already connected then nothing is done and nil
 * is not already connected, a connection will first be established
 * (including auth and refreshing tokens) and then the reader and
 * writers are started.   SendRequest can be called to send requests
 * to the peer and the (user provided) msgChannel will be used to
 * handle messages from the server.
 */
func (ch *ReaderChan[T]) Stop() error {
	if !ch.IsRunning() {
		// already running do nothing
		return nil
	}
	ch.controlChannel <- "stop"
	return nil
}

func (rc *ReaderChan[R]) start() (err error) {
	if rc.IsRunning() {
		return errors.New("Channel already running")
	}
	rc.msgChannel = make(chan ValueOrError[R])
	rc.ReaderWriterBase.start()
	defer func() {
		if rc.OnClose != nil {
			rc.OnClose()
		}
	}()
	go func() {
		ticker := time.NewTicker((rc.WaitTime * 9) / 10)
		defer rc.ReaderWriterBase.cleanup()
		for {
			select {
			case <-rc.controlChannel:
				// For now only a "kill" can be sent here
				return
			case <-ticker.C:
				// TODO - use to send pings
			}
		}
	}()

	// and the actual reader
	for {
		newMessage, err := rc.Read()
		rc.msgChannel <- ValueOrError[R]{
			Value: newMessage,
			Error: err,
		}
		if err != nil {
			break
		}
	}
	return nil
}
