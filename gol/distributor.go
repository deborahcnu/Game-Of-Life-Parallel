package gol

import (
	"fmt"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	keyPresses <-chan rune 
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	quit := false // These flags keep track of whether 'p' or 'q'
	pause := false // (pause or quit) have been pressed
	quitWhilePaused := false // this flag keeps track of whether quit happens while the processing is paused or not
	// because if they both happen, the current world is saved so the new world doesn't need to be generated
	// TODO: Create a 2D slice to store the world.
	world := make([][]uint8, p.ImageHeight)
	for i := 0; i < p.ImageHeight; i++ {
		world[i] = make([]uint8, p.ImageWidth)
	}

	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprintf("%dx%d", p.ImageWidth, p.ImageHeight)

	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			world[i][j] = <-c.ioInput
		}
	}

	turn := 0
	c.events <- StateChange{turn, Executing}

	// TODO: Execute all turns of the Game of Life.

	// create a ticker that will report the number of alive cells every 2 seconds
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop() // stop the ticker once the surrounding function (distributor) finishes

	for turn < p.Turns  {
		select {
		case <- ticker.C:  // 2 second signal reeceived so report number of alive cells
			aliveCells := len(calcAliveCells(p, world))
			c.events <- AliveCellsCount{turn, aliveCells}
		
		case key := <- c.keyPresses: // a key has been pressed so check which one it was
			quit, pause = handleKeyPress(p, c, world, turn, key, quit, pause) // the helper function will handle each case ('s', 'q' or 'p')
			// and will return updated values of the states of quit and pause 
			
			if pause { // if p has been pressed once:
				c.events <- StateChange{turn, Paused} // notify SDL processing has been paused, remain this way until another key is pressed

				for pause && !quit { // we must listen for keypresses while paused
					key := <-c.keyPresses
					quit, pause = handleKeyPress(p, c, world, turn, key, quit, pause) 

					if quit { // if 'q' is pressed while processing is still paused...
						quitWhilePaused = true // don't calculate the next world since the current one is already complete
					}
				}

				if !quit { // if 'p' is pressed again, and 'q' wasn't:
					c.events <- StateChange{turn, Executing} // resume processing
				}

			}

		default: // in this case, the ticker hasn't received a signal so don't do anything for now

		} // end of select statement
		
		if quitWhilePaused { // since 'q' was pressed while processing is still paused, don't get to the end of the turn. Save and quit
			c.events <- FinalTurnComplete{turn, calcAliveCells(p, world)}
        	saveWorld(p, c, world, turn)
        	c.events <- StateChange{turn, Quitting}
        	close(c.events)
			return
		}

		mainWork := p.ImageHeight / p.Threads
		extraWork := p.ImageHeight % p.Threads

		workerOutputs := make([]chan [][]uint8, p.Threads)
		startY := 0

		for i := 0; i < p.Threads; i++ {
			workerRows := mainWork
			if i < extraWork {
				workerRows++
			}
			workerOutputs[i] = make(chan [][]uint8, 1)
			go worker(p, world, startY, startY+workerRows, workerOutputs[i])
			startY += workerRows
		}

		newWorld := make([][]uint8, 0, p.ImageHeight)
		for i := 0; i < p.Threads; i++ {
			workerResult := <-workerOutputs[i]
			newWorld = append(newWorld, workerResult...)
		}

		world = newWorld
		turn++

		if quit { // we only get here if 'q' was pressed while processing was happening, NOT if it was paused
			c.events <- FinalTurnComplete{turn, calcAliveCells(p, world)} // so we finish the current generation
			saveWorld(p, c, world, turn) // save the state of the world as PGM image and then 
			c.events <- StateChange{turn, Quitting} 
			close(c.events)
			return // terminate program
		}
	}

	// Report the final state using FinalTurnCompleteEvent.
	c.events <- FinalTurnComplete{turn, calcAliveCells(p, world)}
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}


func calcAliveCells(p Params, world [][]uint8) []util.Cell {
	LiveCell := []util.Cell{}
	//create empty list using util.cell type to help store the coordinates of a cell
	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			//cycles the board of the new world checking whether each specific cell is alive or dead aka white or black
			if world[i][j] == 255 {
				// if true add coordinations of live cell tso slice
				LiveCell = append(LiveCell, util.Cell{j, i})
			}
		}
	}
	//return list of liveCells
	return LiveCell
}

func worker(p Params, world [][]uint8, startY, endY int, out chan<- [][]uint8) {
	workerArea := make([][]uint8, endY-startY)
	for y := startY; y < endY; y++ { // loop through every cell in the current world
		workerArea[y-startY] = make([]uint8, p.ImageWidth)
		for x := 0; x < p.ImageWidth; x++ {
			liveNeighbours := 0
			for dy := -1; dy <= 1; dy++ { // moving vertically through the neighbours
				for dx := -1; dx <= 1; dx++ { // moving horizontally through the neighbours

					if dy == 0 && dx == 0 { // don't count the current cell
						continue // as a neighbouring cell
					}
					if world[(y+dy+p.ImageHeight)%p.ImageHeight][(x+dx+p.ImageWidth)%p.ImageWidth] == 255 {
						liveNeighbours++ // so add to the live neighbours counter
					}
				}
			}
			if (world[y][x] == 255) && (liveNeighbours == 2 || liveNeighbours == 3) {
				workerArea[y-startY][x] = 255
			} else if (world[y][x] == 0) && (liveNeighbours == 3) {
				workerArea[y-startY][x] = 255 // a dead cell becomes alive again if it has exactly 3 live neighbours
			}
		}
	}

	out <- workerArea
}
// this helper function sends the current state of the world to the I/O goroutine so that it can be written to a PGM image.
func saveWorld(p Params, c distributorChannels, world [][]uint8, turn int) { // also made to avoid reusing code in the switch statement
    // added turn as a parameter since it doesn't exist yet in this function and is needed for ImageOutputComplete event
	c.ioCommand <- ioOutput // telling the I/O goroutine that an image needs to be written
	c.ioFilename <- fmt.Sprintf("%dx%dx%d", p.ImageWidth, p.ImageHeight, turn) // provide a filename based on dimensions and current turn
	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			c.ioOutput <- world[i][j] // sending the current world pixel values to be written to the PGM file
		}
	}
	c.ioCommand <- ioCheckIdle // Make sure that the I/O has finished any output before continuing.
	<-c.ioIdle // blocks at this line until response has been received
	c.events <- ImageOutputComplete{turn, fmt.Sprintf("%dx%dx%d", p.ImageWidth, p.ImageHeight, turn)} // event comfirming that PGM image has been saved
}

// this helper function processes keyboard input and performs the right action depending on whether S, Q or P was pressed.
func handleKeyPress(p Params, c distributorChannels, world [][]uint8, turn int, key rune, quit bool, pause bool) (bool, bool) {
    switch key { // with a switch statement
	case 's': 
		saveWorld(p, c, world, turn) // save the state of world as PGM image and continue processing
	case 'q':
		quit = true // set the flag to be returned to and used by the distributor
	case 'p':
		pause = !pause // this acts like an 'on-and-off' switch/toggle: pressing P pauses execution and pressing P again resumes it.
	}
	return quit, pause // every time this function is called these values are updated so the distributor knows whether it should pause or continue processing
}
