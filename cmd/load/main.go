package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"
)

func main() {
	url := flag.String("url", "http://localhost", "public origin, the Caddy edge")
	painters := flag.Int("painters", 20, "devices that submit and vote")
	viewers := flag.Int("viewers", 20, "devices that hold the event stream")
	hold := flag.Duration("hold", 3*time.Second, "how long viewers stay connected")
	flag.Parse()

	var wg sync.WaitGroup
	for i := 0; i < *viewers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{}
			ctx, cancel := context.WithTimeout(context.Background(), *hold)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, *url+"/v1/events", nil)
			res, err := client.Do(req)
			if err != nil {
				log.Printf("viewer: %v", err)
				return
			}
			defer res.Body.Close()
			_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
		}()
	}

	ok := 0
	var mu sync.Mutex
	for i := 0; i < *painters; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if err := paint(*url, n); err != nil {
				log.Printf("painter %d: %v", n, err)
				return
			}
			mu.Lock()
			ok++
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	fmt.Printf("painters_ok=%d viewers=%d\n", ok, *viewers)
}

func paint(origin string, n int) error {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second}
	token, err := mint(origin, client)
	if err != nil {
		return err
	}
	reqCur, _ := http.NewRequest(http.MethodGet, origin+"/v1/rounds/current", nil)
	reqCur.Header.Set("Authorization", "Bearer "+token)
	res, err := client.Do(reqCur)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("current: %s %s", res.Status, body)
	}
	word := letterWord(n)
	req, _ := http.NewRequest(http.MethodPost, origin+"/v1/rounds/"+roundOf(body)+"/words", strings.NewReader(fmt.Sprintf(`{"text":%q,"seq":1}`, word)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err = client.Do(req)
	if err != nil {
		return err
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("word: %s %s", res.Status, b)
	}
	var created struct {
		WordID string `json:"word_id"`
	}
	_ = json.Unmarshal(b, &created)
	listReq, _ := http.NewRequest(http.MethodGet, origin+"/v1/rounds/"+roundOf(body)+"/words", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRes, err := client.Do(listReq)
	if err != nil {
		return err
	}
	listBody, _ := io.ReadAll(listRes.Body)
	listRes.Body.Close()
	var list struct {
		Words []struct {
			ID string `json:"id"`
		} `json:"words"`
	}
	_ = json.Unmarshal(listBody, &list)
	for _, w := range list.Words {
		if w.ID == "" || w.ID == created.WordID {
			continue
		}
		vote, _ := http.NewRequest(http.MethodPut, origin+"/v1/rounds/"+roundOf(body)+"/vote", strings.NewReader(fmt.Sprintf(`{"word_id":%q,"seq":2}`, w.ID)))
		vote.Header.Set("Content-Type", "application/json")
		vote.Header.Set("Authorization", "Bearer "+token)
		vres, err := client.Do(vote)
		if err != nil {
			return err
		}
		vres.Body.Close()
		return nil
	}
	return nil
}

func letterWord(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	s := "w"
	n++
	for n > 0 {
		s += string(letters[(n-1)%26])
		n = (n - 1) / 26
	}
	return s
}

func roundOf(body []byte) string {
	s := string(body)
	i := strings.Index(s, `"round_id":`)
	if i < 0 {
		return "0"
	}
	s = s[i+len(`"round_id":`):]
	j := 0
	for j < len(s) && (s[j] == ' ' || s[j] == '\n') {
		j++
	}
	k := j
	for k < len(s) && s[k] >= '0' && s[k] <= '9' {
		k++
	}
	return s[j:k]
}
