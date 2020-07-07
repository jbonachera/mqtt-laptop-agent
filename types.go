package main

import homie "github.com/jbonachera/homie-go/homie"

type Provider interface {
	Register(homie.Device)
}
