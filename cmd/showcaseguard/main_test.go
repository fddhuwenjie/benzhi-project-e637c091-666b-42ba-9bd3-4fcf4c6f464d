package main

import "testing"

func TestAddressConfiguration(t *testing.T) {
	tests := []struct {
		name, port string
		args       []string
		want       string
		fails      bool
	}{
		{name: "default", want: "127.0.0.1:19081"},
		{name: "port environment", port: "19123", want: "127.0.0.1:19123"},
		{name: "flag override", port: "19123", args: []string{"-addr=127.0.0.1:19234"}, want: "127.0.0.1:19234"},
		{name: "reject wildcard", args: []string{"-addr=0.0.0.0:19081"}, fails: true},
		{name: "reject bad port", port: "8080x", fails: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := parseConfig(test.args, func(key string) string {
				if key == "PORT" {
					return test.port
				}
				return ""
			})
			if test.fails {
				if err == nil {
					t.Fatal("应返回错误")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if config.address != test.want {
				t.Fatalf("地址=%s, want %s", config.address, test.want)
			}
		})
	}
}
