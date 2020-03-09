package leaktest

import "fmt"
import "testing"
import "time"
//import "github.com/system-pclub/leaktest"


func TestLeak(t * testing.T) {
	defer AfterTest(t)()
	c := make(chan int)

	go func() {
		time.Sleep(2 * time.Second)
		c <- 1
	}()

	select {
	case <- time.After(1 * time.Second):
		return
	case i := <- c:
			fmt.Printf("%d\n", i)
	}
}