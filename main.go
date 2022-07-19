package main

import (
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"strconv"
	"time"
)

var (
	errNotFound  = errors.New("not found")
	errNoTimeout = errors.New("no timeout")
)

type KeyValue struct {
	key   string
	value []string
}

type MyStack struct {
	data []*KeyValue
}

func (s *MyStack) CreateQueue(key, value string) (*KeyValue, error) {
	newElem := &KeyValue{
		key:   key,
		value: make([]string, 0),
	}
	newElem.value = append(newElem.value, value)
	s.data = append(s.data, newElem)

	return newElem, nil
}

func (s *MyStack) AddInQueue(key, value string) error {
	elem, err := s.findByKey(key)
	if err != nil {
		return err
	}

	elem.value = append(elem.value, value)

	return nil
}

func (s *MyStack) DelValue(key string) (string, error) {
	elem, err := s.findByKey(key)
	if err != nil {
		return "", err
	}

	lastElem := elem.value[len(elem.value)-1]
	elem.value = elem.value[:len(elem.value)-1]

	return lastElem, nil
}

func (s *MyStack) findByKey(key string) (*KeyValue, error) {
	for _, elem := range s.data {
		if elem.key == key {
			return elem, nil
		}
	}
	return nil, errNotFound
}

type QueuesMemoryRepo struct {
	data MyStack
}

func NewMemoryRepo() *QueuesMemoryRepo {
	return &QueuesMemoryRepo{}
}

func (repo *QueuesMemoryRepo) Get(key string) (string, error) {
	elem, err := repo.data.findByKey(key)
	if err != nil {
		return "", err
	}

	if len(elem.value) == 0 {
		return "", errNotFound
	}

	ans, err := repo.data.DelValue(key)
	if err != nil {
		return "", err
	}

	return ans, nil
}

func (repo *QueuesMemoryRepo) Put(key, value string) error {
	_, err := repo.data.findByKey(key)
	if err == errNotFound {
		_, err := repo.data.CreateQueue(key, value)
		if err != nil {
			return err
		}
		return nil
	} else if err != nil {
		return err
	}

	err = repo.data.AddInQueue(key, value)
	if err != nil {
		return err
	}

	return nil
}

type QueuesRepo interface {
	Get(key string) (string, error)
	Put(key, value string) error
}

type QueueHandler struct {
	Repo QueuesRepo
}

func ParseTimeout(r *http.Request) (int64, error) {
	t := r.URL.Query().Get("timeout")
	if len(t) == 0 {
		return 0, errNoTimeout
	}
	timeout, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		return 0, err
	}

	return timeout, nil
}

func (q *QueueHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	timeout, err := ParseTimeout(r)
	if err != nil && err != errNoTimeout {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	res, err := q.Repo.Get(r.URL.Path[1:])

	for i := 0; i < int(timeout) && err != nil; i++ {
		res, err = q.Repo.Get(r.URL.Path[1:])
		time.Sleep(time.Second)
	}

	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(err)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(res)
}

func (q *QueueHandler) HandlePut(w http.ResponseWriter, r *http.Request) {
	pathname := r.URL.Path
	param := r.URL.Query().Get("v")

	if len(param) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(http.StatusBadRequest)
		return
	}

	err := q.Repo.Put(pathname[1:], param)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (q *QueueHandler) Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		q.HandleGet(w, r)
	} else if r.Method == http.MethodPut {
		q.HandlePut(w, r)
	}
}

func main() {
	port := flag.String("port", ":8080", "port")
	flag.Parse()

	queueRepo := NewMemoryRepo()
	queueHandler := &QueueHandler{
		Repo: queueRepo,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", queueHandler.Handler)

	server := &http.Server{
		Addr:    *port,
		Handler: mux,
	}

	server.ListenAndServe()
}
