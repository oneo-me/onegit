package gitview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Commit authors' avatars come from the GitHub account their email belongs
// to, including GitHub's noreply addresses, else from Gravatar. Each is read
// once per session and kept on disk by the hash of the email, so the history
// draws without a request per row: a picture for avatarMaxAge, and the
// answer that an email has none for avatarMissingMaxAge. A stale picture
// stays in use when neither service can be reached.
const (
	avatarMaxAge        = 7 * 24 * time.Hour
	avatarMissingMaxAge = 24 * time.Hour
	// avatarPixels is the size requested, sharp at three times a row's
	// avatar.
	avatarPixels = 64
	// avatarMaxBytes bounds a downloaded picture.
	avatarMaxBytes = 1 << 20
	// avatarFetches bounds the reads running at once.
	avatarFetches = 4
)

// githubUnknownEmail asks GitHub for its default picture, which it answers
// for every email without an account.
const githubUnknownEmail = "nobody@gitview.invalid"

// avatarCache holds the authors' avatars by email hash: a picture, or nil
// while reading or when the author has none.
type avatarCache struct {
	// dir keeps the downloaded pictures; github and gravatar are the
	// services' avatar URLs.
	dir, github, gravatar string
	client                *http.Client
	images                map[string][]byte
	slots                 chan struct{}

	// githubDefault is GitHub's picture for an email without an account,
	// read once it is first needed.
	mu            sync.Mutex
	githubDefault []byte
}

func newAvatarCache(dir string) *avatarCache {
	return &avatarCache{dir: dir, github: "https://avatars.githubusercontent.com/u/e", gravatar: "https://gravatar.com/avatar/",
		client: &http.Client{Timeout: 15 * time.Second}, images: map[string][]byte{}, slots: make(chan struct{}, avatarFetches)}
}

// avatarHash is Gravatar's key for an email: the SHA-256 of its trimmed,
// lowercase form.
func avatarHash(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:])
}

// avatar returns the picture of email, or nil while it is read or when
// there is none. The first request starts reading it in the background;
// the result is applied on the engine, showing the view again.
func (v *View) avatar(email string) []byte {
	ac := v.avatars
	if ac == nil || strings.TrimSpace(email) == "" {
		return nil
	}
	key := avatarHash(email)
	if img, ok := ac.images[key]; ok {
		return img
	}
	ac.images[key] = nil
	go func() {
		ac.slots <- struct{}{}
		data := ac.read(email, time.Now())
		<-ac.slots
		if data == nil {
			return
		}
		if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
			return
		}
		v.dispatch(func() {
			ac.images[key] = data
			v.invalidate()
		})
	}()
	return nil
}

// read returns the picture of email from the disk cache while fresh, else
// from GitHub or Gravatar, recording their answer; nil means there is none.
func (ac *avatarCache) read(email string, now time.Time) []byte {
	key := avatarHash(email)
	picture, missing := filepath.Join(ac.dir, key+".picture"), filepath.Join(ac.dir, key+".missing")
	// Earlier versions asked Gravatar alone; their answers are dropped.
	os.Remove(filepath.Join(ac.dir, key))
	os.Remove(filepath.Join(ac.dir, key+".none"))
	cached, _ := os.ReadFile(picture)
	if info, err := os.Stat(picture); err == nil && len(cached) > 0 && now.Sub(info.ModTime()) < avatarMaxAge {
		return cached
	}
	if info, err := os.Stat(missing); err == nil && now.Sub(info.ModTime()) < avatarMissingMaxAge {
		return nil
	}
	data, err := ac.download(email)
	switch {
	case errors.Is(err, errNoAvatar):
		os.Remove(picture)
		if os.MkdirAll(ac.dir, 0700) == nil {
			os.WriteFile(missing, nil, 0600)
		}
		return nil
	case err != nil:
		return cached
	}
	os.Remove(missing)
	if os.MkdirAll(ac.dir, 0700) == nil {
		writeFileAtomic(picture, data)
	}
	return data
}

var errNoAvatar = errors.New("no avatar")

// download returns the picture of email from GitHub, else Gravatar,
// answering errNoAvatar only when both answered that there is none.
func (ac *avatarCache) download(email string) ([]byte, error) {
	data, githubErr := ac.downloadGitHub(email)
	if githubErr == nil {
		return data, nil
	}
	data, err := ac.fetch(fmt.Sprintf("%s%s?s=%d&d=404", ac.gravatar, avatarHash(email), avatarPixels))
	if err == nil || !errors.Is(err, errNoAvatar) || errors.Is(githubErr, errNoAvatar) {
		return data, err
	}
	return nil, githubErr
}

// downloadGitHub asks GitHub for the picture of the account email belongs
// to, answering errNoAvatar for its default picture.
func (ac *avatarCache) downloadGitHub(email string) ([]byte, error) {
	none, err := ac.githubNone()
	if err != nil {
		return nil, err
	}
	data, err := ac.fetch(ac.githubURL(email))
	if err == nil && bytes.Equal(data, none) {
		return nil, errNoAvatar
	}
	return data, err
}

// githubNone returns GitHub's default picture, reading it on first use.
func (ac *avatarCache) githubNone() ([]byte, error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.githubDefault == nil {
		data, err := ac.fetch(ac.githubURL(githubUnknownEmail))
		if err != nil {
			return nil, err
		}
		ac.githubDefault = data
	}
	return ac.githubDefault, nil
}

func (ac *avatarCache) githubURL(email string) string {
	return fmt.Sprintf("%s?email=%s&s=%d", ac.github, url.QueryEscape(strings.TrimSpace(email)), avatarPixels)
}

// fetch downloads the picture at address, answering errNoAvatar when the
// service has none.
func (ac *avatarCache) fetch(address string) ([]byte, error) {
	resp, err := ac.client.Get(address)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, errNoAvatar
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("avatar: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, avatarMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > avatarMaxBytes {
		return nil, errors.New("avatar: picture too large")
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("avatar: %w", err)
	}
	return data, nil
}

// writeFileAtomic replaces path with data, so a reader never sees a partial
// file.
func writeFileAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".avatar-*")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}

// avatarStack shows the author's picture in front of up to two co-authors'
// pictures, each peeking out a little further to the right and ringed in
// the window's background to set it apart from the one behind.
func (v *View) avatarStack(cx *core.Scope, commit Commit, size core.Points) core.Node {
	co := commit.CoAuthors[:min(len(commit.CoAuthors), 2)]
	if len(co) == 0 {
		return v.avatarView(cx, commit.Author, commit.AuthorEmail, size)
	}
	colors := ui.ColorsOf(cx)
	step := size * 0.35
	ringed := func(name, email string, left core.Points) core.Node {
		return core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(left), CornerRadius: size/2 + 1, BorderWidth: 1},
			StyleFunc: func() core.Style {
				return core.Style{BorderColor: colors.Background(), Background: colors.Background()}
			},
			Children: []core.Node{v.avatarView(cx, name, email, size)}}
	}
	// Absolute children paint in order, so the backmost comes first.
	var layers []core.Node
	for i := len(co) - 1; i >= 0; i-- {
		layers = append(layers, ringed(co[i].Name, co[i].Email, step*core.Points(i+1)))
	}
	layers = append(layers, ringed(commit.Author, commit.AuthorEmail, 0))
	return core.View{Style: core.Style{Width: core.Pt(size + step*core.Points(len(co)) + 2), Height: core.Pt(size + 2), FlexShrink: 0}, Children: layers}
}

// avatarView is a commit author's avatar, size wide, or their initials
// until it is read or when there is none.
func (v *View) avatarView(cx *core.Scope, name, email string, size core.Points) core.Node {
	picture := core.NewMemoFunc(cx, func() []byte { v.track(); return v.avatar(email) }, func(a, b []byte) bool { return bytes.Equal(a, b) })
	return core.Dynamic(func(cx *core.Scope) core.Node {
		data := picture.Get()
		a := ui.Avatar{Name: name, Size: size, Style: core.Style{FlexShrink: 0}}
		if data != nil {
			a.Image = core.ImageSource{Data: data}
		}
		return a
	})
}
