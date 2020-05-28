// Copyright 2013 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var robotTests = []string{
	"Mozilla/5.0 (compatible; TweetedTimes Bot/1.0; +http://tweetedtimes.com)",
	"Mozilla/5.0 (compatible; YandexBot/3.0; +http://yandex.com/bots)",
	"Mozilla/5.0 (compatible; MJ12bot/v1.4.3; http://www.majestic12.co.uk/bot.php?+)",
	"Go 1.1 package http",
	"Java/1.7.0_25	0.003	0.003",
	"Python-urllib/2.6",
	"Mozilla/5.0 (compatible; archive.org_bot +http://www.archive.org/details/archive.org_bot)",
	"Mozilla/5.0 (compatible; Ezooms/1.0; ezooms.bot@gmail.com)",
	"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
}

func TestRobotPat(t *testing.T) {
	// TODO(light): isRobot checks for more than just the User-Agent.
	// Extract out the database interaction to an interface to test the
	// full analysis.

	for _, tt := range robotTests {
		if !robotPat.MatchString(tt) {
			t.Errorf("%s not a robot", tt)
		}
	}
}

func TestHandlePkgGoDevRedirect(t *testing.T) {
	handler := pkgGoDevRedirectHandler(func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	for _, test := range []struct {
		name, url, wantLocationHeader, wantSetCookieHeader string
		wantStatusCode                                     int
		cookie                                             *http.Cookie
	}{
		{
			name:                "test pkggodev-redirect param is on",
			url:                 "http://godoc.org/net/http?redirect=on",
			wantLocationHeader:  "https://pkg.go.dev/net/http?tab=doc&utm_source=godoc",
			wantSetCookieHeader: "pkggodev-redirect=on; Path=/",
			wantStatusCode:      http.StatusFound,
		},
		{
			name:                "test pkggodev-redirect param is off",
			url:                 "http://godoc.org/net/http?redirect=off",
			wantLocationHeader:  "",
			wantSetCookieHeader: "pkggodev-redirect=; Path=/; Max-Age=0",
			wantStatusCode:      http.StatusOK,
		},
		{
			name:                "test pkggodev-redirect param is unset",
			url:                 "http://godoc.org/net/http",
			wantLocationHeader:  "",
			wantSetCookieHeader: "",
			wantStatusCode:      http.StatusOK,
		},
		{
			name:                "toggle enabled pkggodev-redirect cookie",
			url:                 "http://godoc.org/net/http?redirect=off",
			cookie:              &http.Cookie{Name: "pkggodev-redirect", Value: "true"},
			wantLocationHeader:  "",
			wantSetCookieHeader: "pkggodev-redirect=; Path=/; Max-Age=0",
			wantStatusCode:      http.StatusOK,
		},
		{
			name:                "pkggodev-redirect enabled cookie should redirect",
			url:                 "http://godoc.org/net/http",
			cookie:              &http.Cookie{Name: "pkggodev-redirect", Value: "on"},
			wantLocationHeader:  "https://pkg.go.dev/net/http?tab=doc&utm_source=godoc",
			wantSetCookieHeader: "",
			wantStatusCode:      http.StatusFound,
		},
		{
			name:           "do not redirect if user is returning from pkg.go.dev",
			url:            "http://godoc.org/net/http?utm_source=backtogodoc",
			cookie:         &http.Cookie{Name: "pkggodev-redirect", Value: "on"},
			wantStatusCode: http.StatusOK,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", test.url, nil)
			if test.cookie != nil {
				req.AddCookie(test.cookie)
			}

			w := httptest.NewRecorder()
			err := handler(w, req)
			if err != nil {
				t.Fatal(err)
			}
			resp := w.Result()

			if got, want := resp.Header.Get("Location"), test.wantLocationHeader; got != want {
				t.Errorf("Location header mismatch: got %q; want %q", got, want)
			}

			if got, want := resp.Header.Get("Set-Cookie"), test.wantSetCookieHeader; got != want {
				t.Errorf("Set-Cookie header mismatch: got %q; want %q", got, want)
			}

			if got, want := resp.StatusCode, test.wantStatusCode; got != want {
				t.Errorf("Status code mismatch: got %d; want %d", got, want)
			}
		})
	}
}

func TestGodoc(t *testing.T) {
	testCases := []struct {
		from, to string
	}{
		{
			from: "https://godoc.org/-/about",
			to:   "https://pkg.go.dev/about?utm_source=godoc",
		},
		{
			from: "https://godoc.org/-/go",
			to:   "https://pkg.go.dev/std?tab=packages&utm_source=godoc",
		},
		{
			from: "https://godoc.org/?q=foo",
			to:   "https://pkg.go.dev/search?q=foo&utm_source=godoc",
		},
		{
			from: "https://godoc.org/cloud.google.com/go/storage",
			to:   "https://pkg.go.dev/cloud.google.com/go/storage?tab=doc&utm_source=godoc",
		},
		{
			from: "https://godoc.org/cloud.google.com/go/storage?imports",
			to:   "https://pkg.go.dev/cloud.google.com/go/storage?tab=imports&utm_source=godoc",
		},
		{
			from: "https://godoc.org/cloud.google.com/go/storage?importers",
			to:   "https://pkg.go.dev/cloud.google.com/go/storage?tab=importedby&utm_source=godoc",
		},
	}

	for _, tc := range testCases {
		u, err := url.Parse(tc.from)
		if err != nil {
			t.Errorf("url.Parse(%q): %v", tc.from, err)
			continue
		}
		to := pkgGoDevURL(u)
		if got, want := to.String(), tc.to; got != want {
			t.Errorf("pkgGoDevURL(%q) = %q; want %q", u, got, want)
		}
	}
}

func TestNewGDDOEvent(t *testing.T) {
	for _, test := range []struct {
		url  string
		want *gddoEvent
	}{
		{
			url: "https://godoc.org",
			want: &gddoEvent{
				Host: "godoc.org",
				Path: "",
			},
		},
		{
			url: "https://godoc.org/-/about",
			want: &gddoEvent{
				Host: "godoc.org",
				Path: "/-/about",
			},
		},
		{
			url: "https://godoc.org/?q=test",
			want: &gddoEvent{
				Host: "godoc.org",
				Path: "/",
			},
		},
		{
			url: "https://godoc.org/net/http",
			want: &gddoEvent{
				Host: "godoc.org",
				Path: "/net/http",
			},
		},
		{
			url: "https://api.godoc.org/imports/net/http",
			want: &gddoEvent{
				Host: "api.godoc.org",
				Path: "/imports/net/http",
			},
		},
	} {
		t.Run(test.url, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			want := test.want
			want.Latency = 100
			want.RedirectHost = "https://" + pkgGoDevHost
			want.URL = test.url
			want.Header = http.Header{}
			want.IsRobot = true
			got := newGDDOEvent(r, want.Latency, want.IsRobot)
			if diff := cmp.Diff(want, got); diff != "" {
				t.Fatalf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
