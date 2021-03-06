package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/codegangsta/cli"
	"github.com/exercism/cli/config"
	"github.com/exercism/cli/handlers"
)

const (
	// Version is the current release of the command-line app.
	// We try to follow Semantic Versioning (http://semver.org),
	// but with the http://exercism.io app being a prototype, a
	// lot of things get out of hand.
	Version = "1.7.0"

	msgPleaseAuthenticate = "You must be authenticated. Run `exercism configure --key=YOUR_API_KEY`."

	descDebug     = "Outputs useful debug information."
	descConfigure = "Writes config values to a JSON file."
	descDemo      = "Fetches a demo problem for each language track on exercism.io."
	descFetch     = "Fetches your current problems on exercism.io, as well as the next unstarted problem in each language."
	descRestore   = "Restores completed and current problems on from exercism.io, along with your most recent iteration for each."
	descSubmit    = "Submits a new iteration to a problem on exercism.io."
	descUnsubmit  = "Deletes the most recently submitted iteration."
	descLogin     = "DEPRECATED: Interactively saves exercism.io api credentials."
	descLogout    = "DEPRECATED: Clear exercism.io api credentials"

	descLongRestore = "Restore will pull the latest revisions of exercises that have already been submitted. It will *not* overwrite existing files. If you have made changes to a file and have not submitted it, and you're trying to restore the last submitted version, first move that file out of the way, then call restore."
)

var (
	// UserAgent is sent along as a header to HTTP requests that the
	// CLI makes. This helps with debugging.
	UserAgent = fmt.Sprintf("github.com/exercism/cli v%s (%s/%s)", Version, runtime.GOOS, runtime.GOARCH)
)

var FetchEndpoints = map[string]string{
	"current":  "/api/v1/user/assignments/current",
	"next":     "/api/v1/user/assignments/next",
	"restore":  "/api/v1/user/assignments/restore",
	"exercise": "/api/v1/assignments",
}

var testExtensions = map[string]string{
	"ruby":    "_test.rb",
	"js":      ".spec.js",
	"elixir":  "_test.exs",
	"clojure": "_test.clj",
	"python":  "_test.py",
	"go":      "_test.go",
	"haskell": "_test.hs",
	"cpp":     "_test.cpp",
}

func main() {
	app := cli.NewApp()
	app.Name = "exercism"
	app.Usage = "A command line tool to interact with http://exercism.io"
	app.Version = Version
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "config, c",
			Usage: "path to config file",
		},
	}
	app.Commands = []cli.Command{
		{
			Name:   "debug",
			Usage:  descDebug,
			Action: handlers.Debug,
		},
		{
			Name:  "configure",
			Usage: descConfigure,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "dir, d",
					Usage: "path to exercises directory",
				},
				cli.StringFlag{
					Name:  "host, u",
					Usage: "exercism api host",
				},
				cli.StringFlag{
					Name:  "key, k",
					Usage: "exercism.io API key (see http://exercism.io/account)",
				},
			},
			Action: handlers.Configure,
		},
		{
			Name:      "demo",
			ShortName: "d",
			Usage:     descDemo,
			Action:    handlers.Demo,
		},
		{
			Name:      "fetch",
			ShortName: "f",
			Usage:     descFetch,
			Action:    handlers.Fetch,
		},
		{
			Name:      "login",
			ShortName: "l",
			Usage:     descLogin,
			Action:    handlers.Login,
		},
		{
			Name:      "logout",
			ShortName: "o",
			Usage:     descLogout,
			Action:    handlers.Logout,
		},
		{
			Name:        "restore",
			ShortName:   "r",
			Usage:       descRestore,
			Description: descLongRestore,
			Action:      handlers.Restore,
		},
		{
			Name:      "submit",
			ShortName: "s",
			Usage:     descSubmit,
			Action: func(ctx *cli.Context) {
				if len(ctx.Args()) == 0 {
					fmt.Println("Please enter a file name")
					return
				}

				c, err := config.Read(ctx.GlobalString("config"))
				if err != nil {
					fmt.Println(err)
					return
				}

				if !c.IsAuthenticated() {
					fmt.Println(msgPleaseAuthenticate)
					return
				}

				filename := ctx.Args()[0]

				// Make filename relative to config.Dir.
				absPath, err := absolutePath(filename)
				if err != nil {
					fmt.Printf("Couldn't find %v: %v\n", filename, err)
					return
				}
				exDir := c.Dir + string(filepath.Separator)
				if !strings.HasPrefix(absPath, exDir) {
					fmt.Printf("%v is not under your exercism project path (%v)\n", absPath, exDir)
					return
				}
				filename = absPath[len(exDir):]

				if IsTest(filename) {
					fmt.Println("It looks like this is a test, please submit a solution.")
					return
				}

				code, err := ioutil.ReadFile(absPath)
				if err != nil {
					fmt.Printf("Error reading %v: %v\n", absPath, err)
					return
				}

				response, err := SubmitAssignment(c, filename, code)
				if err != nil {
					fmt.Printf("There was an issue with your submission: %v\n", err)
					return
				}

				fmt.Printf("For feedback on your submission visit %s%s%s\n",
					c.Hostname, "/submissions/", response.ID)

			},
		},
		{
			Name:      "unsubmit",
			ShortName: "u",
			Usage:     descUnsubmit,
			Action: func(ctx *cli.Context) {
				c, err := config.Read(ctx.GlobalString("config"))
				if err != nil {
					fmt.Println(err)
					return
				}

				if !c.IsAuthenticated() {
					fmt.Println(msgPleaseAuthenticate)
					return
				}

				err = UnsubmitAssignment(c)
				if err != nil {
					fmt.Println(err)
					return
				}
				fmt.Println("The last submission was successfully deleted.")
			},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		fmt.Errorf("%v", err)
		os.Exit(1)
	}
}

func absolutePath(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}

type submitResponse struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	Language       string `json:"language"`
	Exercise       string `json:"exercise"`
	SubmissionPath string `json:"submission_path"`
	Error          string `json:"error"`
}

type submitRequest struct {
	Key  string `json:"key"`
	Code string `json:"code"`
	Path string `json:"path"`
}

func FetchAssignments(c *config.Config, path string) ([]Assignment, error) {
	url := fmt.Sprintf("%s%s?key=%s", c.Hostname, path, c.APIKey)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		err = fmt.Errorf("Error fetching assignments: [%v]", err)
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	if err != nil {
		err = fmt.Errorf("Error fetching assignments: [%v]", err)
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		var apiError struct {
			Error string `json:"error"`
		}
		err = json.Unmarshal(body, &apiError)
		if err != nil {
			err = fmt.Errorf("Error parsing API response: [%v]", err)
			return nil, err
		}

		err = fmt.Errorf("Error fetching assignments. HTTP Status Code: %d\n%s", resp.StatusCode, apiError.Error)
		return nil, err
	}

	var fr struct {
		Assignments []Assignment
	}

	err = json.Unmarshal(body, &fr)
	if err != nil {
		err = fmt.Errorf("Error parsing API response: [%v]", err)
		return nil, err
	}

	return fr.Assignments, nil
}

func UnsubmitAssignment(c *config.Config) error {
	path := "api/v1/user/assignments"

	url := fmt.Sprintf("%s/%s?key=%s", c.Hostname, path, c.APIKey)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", UserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		err = fmt.Errorf("Error destroying submission: [%v]", err)
		return err
	}

	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusNoContent {

		var ur struct {
			Error string
		}

		err = json.Unmarshal(body, &ur)
		if err != nil {
			return err
		}

		err = fmt.Errorf("Status: %d, Error: %v", resp.StatusCode, ur.Error)
		return err
	}

	return nil
}
func SubmitAssignment(c *config.Config, filePath string, code []byte) (*submitResponse, error) {
	path := "api/v1/user/assignments"

	url := fmt.Sprintf("%s/%s", c.Hostname, path)

	submission := submitRequest{Key: c.APIKey, Code: string(code), Path: filePath}
	submissionJSON, err := json.Marshal(submission)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(submissionJSON))
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		err = fmt.Errorf("Error posting assignment: [%v]", err)
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}

	var r submitResponse
	if resp.StatusCode != http.StatusCreated {
		err = json.Unmarshal(body, &r)
		if err != nil {
			return nil, err
		}
		err = fmt.Errorf("Status: %d, Error: %v", resp.StatusCode, r)
		return nil, err
	}

	err = json.Unmarshal(body, &r)
	if err != nil {
		return nil, fmt.Errorf("Error parsing API response: [%v]", err)
	}
	return &r, nil
}

type Assignment struct {
	Track   string
	Slug    string
	Files   map[string]string
	IsFresh bool `json:"fresh"`
}

func SaveAssignment(dir string, a Assignment) error {
	root := fmt.Sprintf("%s/%s/%s", dir, a.Track, a.Slug)

	for name, text := range a.Files {
		file := fmt.Sprintf("%s/%s", root, name)
		dir := filepath.Dir(file)
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return fmt.Errorf("Error making directory %v: [%v]", dir, err)
		}
		if _, err = os.Stat(file); err != nil {
			if os.IsNotExist(err) {
				err = ioutil.WriteFile(file, []byte(text), 0644)
				if err != nil {
					return fmt.Errorf("Error writing file %v: [%v]", name, err)
				}
			}
		}
	}

	fresh := " "
	if a.IsFresh {
		fresh = "*"
	}
	fmt.Println(fresh, a.Track, "-", a.Slug)

	return nil
}

func FetchEndpoint(args []string) string {
	if len(args) == 0 {
		return FetchEndpoints["current"]
	}

	endpoint := FetchEndpoints["exercise"]
	for _, arg := range args {
		endpoint = fmt.Sprintf("%s/%s", endpoint, arg)
	}

	return endpoint
}

func IsTest(filename string) bool {
	for _, ext := range testExtensions {
		if strings.LastIndex(filename, ext) > 0 {
			return true
		}
	}
	return false
}
