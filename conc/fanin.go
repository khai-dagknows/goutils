package conc

import "log"

type FanInCmd[T any] struct {
	Name           string
	AddedChannel   <-chan T
	RemovedChannel <-chan T
}

type Input[T any] struct {
	inchan <-chan T
	pipe   *Pipe[T]
}

type FanIn[T any] struct {
	RunnerBase[FanInCmd[T]]
	// Called when a channel is removed so the caller can
	// perform other cleanups etc based on this
	OnChannelRemoved func(fi *FanIn[T], inchan <-chan T)

	inputs     []Input[T]
	selfOwnOut bool
	outChan    chan T
}

func NewFanIn[T any](outChan chan T) *FanIn[T] {
	selfOwnOut := false
	if outChan == nil {
		outChan = make(chan T)
		selfOwnOut = true
	}
	out := &FanIn[T]{
		RunnerBase: NewRunnerBase(FanInCmd[T]{Name: "stop"}),
		outChan:    outChan,
		selfOwnOut: selfOwnOut,
	}
	out.start()
	return out
}

func (fi *FanIn[T]) RecvChan() chan T {
	return fi.outChan
}

func (fi *FanIn[T]) Add(inputs ...<-chan T) {
	for _, input := range inputs {
		if input == nil {
			panic("Cannot add nil channels")
		}
		fi.controlChan <- FanInCmd[T]{Name: "add", AddedChannel: input}
	}
}

/**
 * Remove an input channel from our monitor list.
 */
func (fi *FanIn[T]) Remove(target <-chan T) {
	fi.controlChan <- FanInCmd[T]{Name: "remove", RemovedChannel: target}
}

func (fi *FanIn[T]) Count() int {
	return len(fi.inputs)
}

func (fi *FanIn[T]) cleanup() {
	for _, input := range fi.inputs {
		input.pipe.Stop()
		fi.wg.Done()
	}
	fi.inputs = nil
	if fi.selfOwnOut {
		close(fi.outChan)
	}
	fi.outChan = nil
	fi.RunnerBase.cleanup()
}

func (fi *FanIn[T]) start() {
	fi.RunnerBase.start()
	go func() {
		defer fi.cleanup()
		for {
			cmd := <-fi.controlChan
			if cmd.Name == "stop" {
				return
			} else if cmd.Name == "add" {
				// Add a new reader to our list
				fi.wg.Add(1)
				input := Input[T]{
					pipe:   NewPipe(cmd.AddedChannel, fi.outChan),
					inchan: cmd.AddedChannel,
				}
				fi.inputs = append(fi.inputs, input)
				input.pipe.OnDone = fi.pipeClosed
			} else if cmd.Name == "remove" {
				// Remove an existing reader from our list
				log.Println("Removing channel: ", cmd.RemovedChannel)
				fi.remove(cmd.RemovedChannel)
			}
		}
	}()
}

func (fi *FanIn[T]) removeAt(index int) {
	inchan := fi.inputs[index].inchan
	fi.inputs[index].pipe.Stop()
	fi.inputs[index] = fi.inputs[len(fi.inputs)-1]
	fi.inputs = fi.inputs[:len(fi.inputs)-1]
	if fi.OnChannelRemoved != nil {
		fi.OnChannelRemoved(fi, inchan)
	}
	fi.wg.Done()
}

func (fi *FanIn[T]) pipeClosed(p *Pipe[T]) {
	for index, input := range fi.inputs {
		if input.pipe == p {
			fi.removeAt(index)
			break
		}
	}
}

func (fi *FanIn[T]) remove(inchan <-chan T) {
	for index, input := range fi.inputs {
		if input.inchan == inchan {
			fi.removeAt(index)
			break
		}
	}
}
