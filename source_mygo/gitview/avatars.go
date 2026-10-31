package gitview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
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
	images                map[string]*ui.Bitmap
	slots                 chan struct{}

	// githubDefault is GitHub's picture for an email without an account,
	// read once it is first needed.
	mu            sync.Mutex
	githubDefault []byte
}

func newAvatarCache(dir string) *avatarCache {
	return &avatarCache{dir: dir, github: "https://avatars.githubusercontent.com/u/e", gravatar: "https://gravatar.com/avatar/",
		client: &http.Client{Timeout: 15 * time.Second}, images: map[string]*ui.Bitmap{}, slots: make(chan struct{}, avatarFetches)}
}

// avatarHash is Gravatar's key for an email: the SHA-256 of its trimmed,
// lowercase form.
func avatarHash(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:])
}

// avatar returns the picture of email, or nil while it is read or when there
// is none. The first request starts reading it in the background; post
// applies the result on the UI thread.
func (v *View) avatar(email string) *ui.Bitmap {
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
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return
		}
		bitmap := ui.NewBitmap(img)
		v.dispatch(func() {
			ac.images[key] = bitmap
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
func (v *View) avatarStack(c *ui.Context, commit Commit, size float32) {
	co := commit.CoAuthors[:min(len(commit.CoAuthors), 2)]
	if len(co) == 0 {
		v.avatarView(c, commit.Author, commit.AuthorEmail, size)
		return
	}
	step := size * 0.35
	ring := c.Theme().Background
	ui.Box(c).Size(size+step*float32(len(co)), size).Shrink(0).Children(func() {
		// Absolute children paint in order, so the backmost comes first.
		for i := len(co) - 1; i >= 0; i-- {
			v.avatarView(c, co[i].Name, co[i].Email, size).Border(1, ring).Absolute().Top(0).Left(step * float32(i+1))
		}
		v.avatarView(c, commit.Author, commit.AuthorEmail, size).Border(1, ring).Absolute().Top(0).Left(0)
	})
}

// avatarView draws a commit author's avatar, size wide, or their initial on
// a color picked from the email until it is read or when there is none.
func (v *View) avatarView(c *ui.Context, name, email string, size float32) ui.Element {
	if img := v.avatar(email); img != nil {
		return ui.Image(c, img).Size(size, size).Shrink(0).Fit(ui.Cover).Radius(size / 2).Label(name)
	}
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(strings.TrimSpace(email))))
	color := graphPalette[h.Sum32()%uint32(len(graphPalette))]
	initial := "?"
	for _, r := range strings.TrimSpace(name) {
		initial = string(unicode.ToUpper(r))
		break
	}
	return ui.Box(c).Size(size, size).Shrink(0).Radius(size / 2).Center().Background(color).
		Role(ui.RoleImage).Label(name).Children(func() {
		components.Text(c, initial).SingleLine().FontSize(monogramFontSize(size)).FontWeight(600).TextColor(ui.RGB(255, 255, 255))
	})
}

// monogramFontSize is a whole size for an initial in an avatar of size, so
// avatars of many sizes do not each load another font.
func monogramFontSize(size float32) float32 {
	return float32(math.Round(float64(size * .58)))
}
