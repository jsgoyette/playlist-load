package main

import (
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func NewId(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

type Item struct {
	Id    string `bson:"_id"`
	Path  string `bson:"path"`
	Queue uint32 `bson:"queue"`
	Plays uint32 `bson:"plays"`
}

type FileLoader struct {
	base  string
	paths []string
}

func (f *FileLoader) Load() error {

	// userPath = "/Users/jsgoyette/Data/Downloads"

	if f.base == "" {
		return errors.New("missing file path")
	}

	// check that base exists
	if _, err := os.Stat(f.base); err != nil {
		return err
	}

	// load the files into `paths`
	err := filepath.Walk(f.base, f.visit)

	return err
}

func (f *FileLoader) visit(path string, file os.FileInfo, err error) error {
	if file.Mode().IsRegular() {
		if filepath.Ext(path) == ".mp3" {
			f.paths = append(f.paths, path)
		}
	}
	return nil
}

func (f *FileLoader) Insert(c *mgo.Collection, startingQueue uint32) {

	// for each path found, check if it exists
	// only insert if it does not already exist
	for _, path := range f.paths {

		// skip if already loaded
		if count, _ := c.Find(bson.M{"path": path}).Count(); count > 0 {
			fmt.Printf("%v SKIPPING\n", path)
			continue
		}

		startingQueue++

		item := Item{
			Id:    NewId(18),
			Path:  path,
			Queue: startingQueue,
			Plays: 0,
		}

		fmt.Printf("%v\n", path)

		// insert item
		if err := c.Insert(item); err != nil {
			fmt.Println("could not insert", err)
		}
	}
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func main() {

	flag.Parse()

	f := &FileLoader{
		base: flag.Arg(0),
	}

	// load files from path
	err := f.Load()
	if err != nil {
		fmt.Println("failed to load files:", err)
		os.Exit(1)
	}

	// start mongo connection
	session, err := mgo.Dial("127.0.0.1")
	if err != nil {
		panic(err)
	}
	defer session.Close()

	session.SetMode(mgo.Monotonic, true)
	c := session.DB("playlist").C("items")

	// grab the current highest `queue`
	var highestQueuedItem Item
	err = c.Find(bson.M{}).Sort("-queue").One(&highestQueuedItem)
	if err != nil {
		fmt.Println("could not find highest queue", err)
	}

	f.Insert(c, highestQueuedItem.Queue)

}
