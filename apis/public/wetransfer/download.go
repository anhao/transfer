package wetransfer

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"transfer/apis"
	"transfer/utils"
)

var (
	signDownload string
	regexShorten = regexp.MustCompile("(https://)?we\\.tl/[a-zA-Z0-9\\-]{12}")
	regex        = regexp.MustCompile("(https://)?wetransfer\\.com/downloads/[a-z0-9]{46}/[a-z0-9]{6}")
	regexList    = regexp.MustCompile("{\"id.*}]}")
)

func (b weTransfer) LinkMatcher(v string) bool {
	return regex.MatchString(v) || regexShorten.MatchString(v)
}

func (b weTransfer) DoDownload(link string, config apis.DownConfig) error {
	err := b.download(link, config)
	if err != nil {
		fmt.Printf("download failed on %s, returns %s\n", link, err)
	}
	return nil
}

func (b weTransfer) download(v string, config apis.DownConfig) error {
	client := http.Client{Timeout: time.Duration(b.Config.interval) * time.Second}
	fmt.Printf("fetching ticket..")
	end := utils.DotTicker()

	if !regexShorten.MatchString(v) && !regex.MatchString(v) {
		return fmt.Errorf("url is invaild")
	}
	if config.DebugMode {
		log.Println("step 1/2 metadata")
		log.Printf("link: %+v", v)
	}
	resp, err := client.Get(v)
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	tk := tokenRegex.FindSubmatch(body)
	if len(tk) == 0 {
		return fmt.Errorf("no csrf-token found")
	}
	ticket := requestTicket{
		token:   string(tk[1]),
		cookies: "",
	}
	ck := resp.Header.Values("Set-Cookie")
	for _, v := range ck {
		s := strings.Split(v, ";")
		ticket.cookies += s[0] + ";"
	}
	if config.DebugMode {
		log.Printf("ticket: %+v", ticket)
	}
	_ = resp.Body.Close()
	dat := regexList.FindString(string(body))
	if config.DebugMode {
		log.Printf("dst: %+v", dat)
	}
	var block configBlock
	if err := json.Unmarshal([]byte(dat), &block); err != nil {
		return err
	}
	signDownload = fmt.Sprintf("https://wetransfer.com/api/v4/transfers/%s/download", block.ID)

	*end <- struct{}{}
	fmt.Printf("ok\n")
	if block.State != "downloadable" {
		return fmt.Errorf("link state is not downloadable (state: %s)", block.State)
	}

	for _, item := range block.Item {
		config.Ticket = block.Hash
		err = b.downloadItem(item, ticket, config)
		if err != nil {
			fmt.Println(err)
		}
	}
	return nil
}

func (b weTransfer) downloadItem(item fileInfo, tk requestTicket, config apis.DownConfig) error {
	if config.DebugMode {
		log.Println("step2 -> api/getConf")
	}
	data, _ := json.Marshal(map[string]interface{}{
		"security_hash":  config.Ticket,
		"domain_user_id": utils.GenRandUUID(),
		"file_ids":       []string{item.ID},
	})
	if config.DebugMode {
		log.Printf("tk: %+v", tk)
	}
	resp, err := newRequest(signDownload, string(data), requestConfig{
		action:   "POST",
		debug:    config.DebugMode,
		retry:    0,
		timeout:  time.Duration(b.Config.interval) * time.Second,
		modifier: addToken(tk),
	})
	if err != nil {
		return fmt.Errorf("sign Request returns error: %s, onfile: %s", err, item.Name)
	}

	if config.DebugMode {
		log.Println("step3 -> startDownload")
	}
	filePath, err := filepath.Abs(config.Prefix)
	if err != nil {
		return fmt.Errorf("parse filepath returns error: %s, onfile: %s", err, item.Name)
	}

	if utils.IsExist(filePath) && utils.IsDir(filePath) {
		filePath = path.Join(filePath, item.Name)
	}

	//fmt.Printf("File save to: %s\n", filePath)
	config.Prefix = filePath
	err = apis.DownloadFile(&apis.DownloaderConfig{
		Link:     resp.Download,
		Config:   config,
		Modifier: addHeaders,
	})
	if err != nil {
		return fmt.Errorf("failed DownloaderConfig with error: %s, onfile: %s", err, item.Name)
	}
	return nil
}
