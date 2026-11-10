package gol

import (
	"fmt"

	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

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
	for turn < p.Turns {
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
	}

	// TODO: Report the final state using FinalTurnCompleteEvent.
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

func calculateNextState(p Params, world [][]uint8) [][]uint8 {
	height := p.ImageHeight
	width := p.ImageWidth

	nextWorld := make([][]uint8, len(world)) // create a new world to store the next generation
	for y := range world {                   // loop through every cell in the current world
		nextWorld[y] = make([]uint8, width)
		for x := range world[y] {
			liveNeighbours := 0

			for dy := -1; dy <= 1; dy++ { // moving vertically through the neighbours
				for dx := -1; dx <= 1; dx++ { // moving horizontally through the neighbours

					if dy == 0 && dx == 0 { // don't count the current cell
						continue // as a neighbouring cell
					}

					// wrapping the coordinates around the edge of the world:
					neighbourY := (y + dy + height) % height // calculating the vertical neighbour location
					neighbourX := (x + dx + width) % width   // calculating the horizontal neighbour location

					if world[neighbourY][neighbourX] == 255 { // If a cell has value 255 it's alive
						liveNeighbours++ // so add to the live neighbours counter
					}
				}
			}
			if (world[y][x] == 255) && (liveNeighbours == 2 || liveNeighbours == 3) {
				nextWorld[y][x] = 255 // a live cell survives if it has 2 or 3 live neighbours
			} else if (world[y][x] == 255) && (liveNeighbours < 2 || liveNeighbours > 3) {
				nextWorld[y][x] = 0 // a live cell dies if it has less thsn 2 or greater than 3 live neighbours
			} else if (world[y][x] == 0) && (liveNeighbours == 3) {
				nextWorld[y][x] = 255 // a dead cell becomes alive again if it has exactly 3 live neighbours
			}
		}
	}

	return nextWorld // this function returns the completed next generation
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
