package main

type Storer interface {
	Save(s string) error
}

type MemStore struct{}

func (MemStore) Save(s string) error {
	_ = s
	return nil
}

type FileStore struct{}

func (FileStore) Save(s string) error {
	_ = s
	return nil
}

func use(st Storer) {
	st.Save("hi")
}

func main() {
	use(MemStore{})
}
