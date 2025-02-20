package cowtransfer

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"transfer/apis"
	"transfer/utils"
)

const (
	downloadDetails = "https://cowtransfer.com/transfer/transferdetail?url=%s&treceive=undefined&passcode=%s"
	downloadConfig  = "https://cowtransfer.com/transfer/download?guid=%s"
)

var (
	matcher = regexp.MustCompile("(cowtransfer\\.com|c-t\\.work)/s/[0-9a-f]{14}")
	reg     = regexp.MustCompile("[0-9a-f]{14}")
)

func (b cowTransfer) LinkMatcher(v string) bool {
	return matcher.MatchString(v)
}

func (b cowTransfer) DoDownload(link string, config apis.DownConfig) error {
	return b.initDownload(link, config)
}

func (b cowTransfer) initDownload(v string, config apis.DownConfig) error {
	fileID := reg.FindString(v)
	if config.DebugMode {
		log.Println("starting download...")
		log.Println("step1 -> api/getGuid")
	}
	fmt.Printf("Remote: %s\n", v)
	detailsURL := fmt.Sprintf(downloadDetails, fileID, b.Config.passCode)

	req, err := http.NewRequest("GET", detailsURL, nil)
	if err != nil {
		return fmt.Errorf("createRequest returns error: %s", err)
	}
	addHeaders(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("getDownloadDetails returns error: %s", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("readDownloadDetails returns error: %s", err)
	}

	_ = resp.Body.Close()

	if config.DebugMode {
		log.Printf("returns: %v\n", string(body))
	}
	details := new(downloadDetailsResponse)
	if err := json.Unmarshal(body, details); err != nil {
		return fmt.Errorf("unmatshal DownloadDetails returns error: %s", err)
	}

	if details.GUID == "" {
		return fmt.Errorf("link invalid")
	}

	if details.Deleted {
		return fmt.Errorf("link deleted")
	}

	if !details.Uploaded {
		return fmt.Errorf("link not finish upload yet")
	}

	for _, item := range details.Details {
		err = downloadItem(item, config)
		if err != nil {
			fmt.Println(err)
		}
	}
	return nil
}

func downloadItem(item downloadDetailsBlock, baseConf apis.DownConfig) error {
	if baseConf.DebugMode {
		log.Println("step2 -> api/getConf")
		log.Printf("fileName: %s\n", item.FileName)
		log.Printf("fileSize: %s\n", item.Size)
		log.Printf("GUID: %s\n", item.GUID)
	}
	configURL := fmt.Sprintf(downloadConfig, item.GUID)
	req, err := http.NewRequest("POST", configURL, nil)
	if err != nil {
		return fmt.Errorf("createRequest returns error: %s, onfile: %s", err, item.FileName)
	}
	addHeaders(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("getDownloadConfig returns error: %s, onfile: %s", err, item.FileName)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("readDownloadConfig returns error: %s, onfile: %s", err, item.FileName)
	}

	_ = resp.Body.Close()
	if baseConf.DebugMode {
		log.Printf("returns: %v\n", string(body))
	}
	config := new(downloadConfigResponse)
	if err := json.Unmarshal(body, config); err != nil {
		return fmt.Errorf("unmatshal DownloaderConfig returns error: %s, onfile: %s", err, item.FileName)
	}

	if baseConf.DebugMode {
		log.Println("step3 -> startDownload")
	}
	filePath, err := filepath.Abs(baseConf.Prefix)
	if err != nil {
		return fmt.Errorf("invalid prefix: %s", baseConf.Prefix)
	}

	if utils.IsExist(baseConf.Prefix) {
		if utils.IsFile(baseConf.Prefix) {
			filePath = baseConf.Prefix
		} else {
			filePath = path.Join(baseConf.Prefix, item.FileName)
		}
	}

	//fmt.Printf("File save to: %s\n", filePath)
	baseConf.Prefix = filePath
	err = apis.DownloadFile(&apis.DownloaderConfig{
		Link:     config.Link,
		Config:   baseConf,
		Modifier: addHeaders,
	})
	if err != nil {
		return fmt.Errorf("failed DownloaderConfig with error: %s, onfile: %s", err, item.FileName)
	}
	return nil
}
