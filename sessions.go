/*
midgaard_bot, a Telegram bot which sets a bridge to Midgaard Merc MUD
Copyright (C) 2017 by Javier Sancho Fernandez <jsf at jsancho dot org>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package main

import (
	"log"

	"github.com/google/uuid"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/reiver/go-telnet"
)

type Session struct {
	WsId  uuid.UUID
	Input chan *string
	Error chan error
}

var sessions cmap.ConcurrentMap[string, *Session]
var mercHost string

func initSessions(host string) error {
	sessions = cmap.New[*Session]()
	mercHost = host
	return nil
}

func getSession(wsid uuid.UUID) *Session {
	session, ok := sessions.Get(wsid.String())
	if !ok {
		session = newSession(wsid)
	}
	return session
}

func newSession(wsId uuid.UUID) *Session {
	session := Session{wsId, make(chan *string), make(chan error)}
	sessions.Set(wsId.String(), &session)
	startSession(&session)
	log.Println("Started session for ID:", wsId)
	return &session
}

func startSession(session *Session) {

	go func() {
		telnetInput, telnetOutput, telnetErrorOut, telnetErrorIn := make(chan string), make(chan string), make(chan string), make(chan error)
		caller := TelnetCaller{
			Input:    telnetInput,
			Output:   telnetOutput,
			ErrorOut: telnetErrorOut,
			ErrorIn:  telnetErrorIn,
		}

		go func() {
			for {
				select {
				case evt := <-session.Input:
					log.Default().Println("sending to telnet")
					telnetInput <- *evt
				case body := <-telnetOutput:
					log.Default().Println("sending to ws")
					sendToWs(session.WsId, body)
				case <-telnetErrorOut:
					log.Default().Println("telnet error")
					cancelWs(session.WsId)
					sessions.Remove(session.WsId.String())
					return
				case err := <-session.Error:
					log.Default().Println("ws error")
					telnetErrorIn <- err
					sessions.Remove(session.WsId.String())
				}
			}
		}()

		log.Println("Dialing telnet")
		telnet.DialToAndCall(mercHost, caller)
	}()
}

func sendToSession(session *Session, message *string) {
	session.Input <- message
}

func errorToSession(session *Session, err error) {
	session.Error <- err
}
