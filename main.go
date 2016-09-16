package main

import (
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const alphanum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const alphanumLen = len(alphanum)

func NewId(n int) string {
	bytes := make([]byte, n)
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = alphanum[b%byte(alphanumLen)]
	}
	return string(bytes)
}

var queueStart uint32
var queueMutex *sync.Mutex = &sync.Mutex{}

// playlist item
type Item struct {
	Id    string `bson:"_id"`
	Path  string `bson:"path"`
	Queue uint32 `bson:"queue"`
	Plays uint32 `bson:"plays"`
}

// result is the product of reading a file
type result struct {
	path string
	err  error
}

// walkFiles starts a goroutine to walk the directory tree at root and send the
// path of each regular file on the string channel. It sends the result of the
// walk on the error channel. If done is closed, walkFiles abandons its work.
func walkFiles(root string, done <-chan struct{}) (<-chan string, <-chan error) {

	paths := make(chan string)
	errc := make(chan error, 1)

	go func() {

		// close the paths channel after Walk returns.
		defer close(paths)

		// no select needed for this send, since errc is buffered.
		errc <- filepath.Walk(root, func(path string, info os.FileInfo, err error) error {

			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || filepath.Ext(path) != ".mp3" {
				return nil
			}

			select {
			case paths <- path:
			case <-done:
				return errors.New("walk cancelled")
			}
			return nil
		})
	}()

	return paths, errc
}

// digester reads path names from paths, writes to db and sends the corresponding
// files on c until either paths or done is closed.
func digester(c *mgo.Collection, paths <-chan string, results chan<- result, done <-chan struct{}) {

	for path := range paths {

		// skip if already loaded
		if count, _ := c.Find(bson.M{"path": path}).Count(); count > 0 {
			fmt.Printf("%v SKIPPING\n", path)
			continue
		}

		queueMutex.Lock()
		queueStart++
		queueMutex.Unlock()

		item := Item{
			Id:    NewId(18),
			Path:  path,
			Queue: queueStart,
			Plays: 0,
		}

		fmt.Printf("%v\n", path)

		// insert item
		err := c.Insert(item)
		if err != nil {
			fmt.Println("could not insert", err)
		}

		select {
		case results <- result{path, err}:
		case <-done:
			return
		}
	}
}

func LoadFiles(root string, c *mgo.Collection) error {

	// LoadFiles closes the done channel when it returns; it may do so before
	// receiving all the values from results and errc
	done := make(chan struct{})
	defer close(done)

	paths, errc := walkFiles(root, done)

	// grab the current highest `queue`
	var highestQueuedItem Item
	err := c.Find(bson.M{}).Sort("-queue").One(&highestQueuedItem)
	if err != nil {
		fmt.Println("could not find highest queue", err)
	} else {
		queueStart = highestQueuedItem.Queue
		fmt.Printf("using %v as starting queue\n", queueStart)
	}

	// start a fixed number of goroutines to read and digest files
	const numDigesters = 20
	var wg sync.WaitGroup
	wg.Add(numDigesters)
	results := make(chan result)

	for i := 0; i < numDigesters; i++ {
		go func() {
			digester(c, paths, results, done)
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results {
		if r.err != nil {
			return r.err
		}
	}

	// check whether the walk failed
	if err := <-errc; err != nil {
		return err
	}
	return nil
}

func main() {

	flag.Parse()

	// start mongo connection
	session, err := mgo.Dial("127.0.0.1")
	if err != nil {
		panic(err)
	}
	defer session.Close()

	session.SetMode(mgo.Monotonic, true)
	collection := session.DB("playlist").C("items")

	err = LoadFiles(flag.Arg(0), collection)
	// err = LoadFiles("/Users/jsgoyette/Data/Music", collection)

	if err != nil {
		fmt.Println(err)
	}
}
