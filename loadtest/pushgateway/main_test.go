package main

import (
	"reflect"
	"testing"
)

func TestParseDeviceIDs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fallback string
		want     []string
	}{
		{
			name:     "multiple",
			input:    "d1,d2",
			fallback: "fallback",
			want:     []string{"d1", "d2"},
		},
		{
			name:     "trim and dedupe",
			input:    " d1, d1, , d2 ",
			fallback: "fallback",
			want:     []string{"d1", "d2"},
		},
		{
			name:     "empty fallback",
			input:    "",
			fallback: "fallback",
			want:     []string{"fallback"},
		},
		{
			name:     "commas fallback",
			input:    ",,",
			fallback: "fallback",
			want:     []string{"fallback"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parseDeviceIDs(test.input, test.fallback)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parseDeviceIDs(%q, %q) = %#v, want %#v", test.input, test.fallback, got, test.want)
			}
		})
	}
}

func TestDerivePushMetricsURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "ws", in: "ws://127.0.0.1:11598", want: "http://127.0.0.1:11598/debug/metrics"},
		{name: "wss", in: "wss://push.example/ws?token=x", want: "https://push.example/debug/metrics"},
		{name: "invalid scheme", in: "http://127.0.0.1:11598", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := derivePushMetricsURL(test.in); got != test.want {
				t.Fatalf("derivePushMetricsURL(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}
