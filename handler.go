package main

import (
	"log"
	"net/http"

	"github.com/newrelic/go-agent/v3/newrelic"
)

type Handler struct {
	newRelicApp      *newrelic.Application
	messageProcessor *MessageProcessor
	handler          *http.ServeMux
}

func NewHandler(newRelicApp *newrelic.Application, messageProcessor *MessageProcessor) *Handler {
	h := &Handler{
		newRelicApp:      newRelicApp,
		messageProcessor: messageProcessor,
		handler:          http.NewServeMux(),
	}
	h.handler.HandleFunc(newrelic.WrapHandleFunc(newRelicApp, "/message", h.message))
	return h
}

func (h *Handler) message(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received message request")
	defer r.Body.Close()
	h.messageProcessor.ProcessMessage(r.Body)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) Mux() *http.ServeMux {
	return h.handler
}
