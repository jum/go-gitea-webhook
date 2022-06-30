// Based on: https://github.com/soupdiver/go-gitlab-webhook
// Gitea SDK: https://godoc.org/code.gitea.io/sdk/gitea
// Gitea webhooks: https://docs.gitea.io/en-us/webhooks

package main

import (
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"

	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/sdk/gitea"
)

// ConfigRepository represents a repository from the config file
type ConfigRepository struct {
	Name     string
	Commands []string
}

// Config represents the config file
type Config struct {
	Logfile      string
	Address      string
	Port         int64
	Secret       string
	Repositories []ConfigRepository
}

func panicIf(err error, what ...string) {
	if err != nil {
		if len(what) == 0 {
			panic(err)
		}

		panic(errors.New(err.Error() + (" " + what[0])))
	}
}

var config Config
var configFile string

func main() {
	args := os.Args

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGHUP)

	go func() {
		<-sigc
		config = loadConfig(configFile)
		log.Println("config reloaded")
	}()

	//if we have a "real" argument we take this as conf path to the config file
	if len(args) > 1 {
		configFile = args[1]
	} else {
		configFile = "config.json"
	}

	var err error
	//load config
	config = loadConfig(configFile)

	if config.Logfile != "-" {
		//open log file
		writer, err := os.OpenFile(config.Logfile, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
		panicIf(err)

		//close logfile on exit
		defer func() {
			writer.Close()
		}()

		//setting logging output
		log.SetOutput(writer)
	}

	//setting handler
	http.Handle("/", gitea.VerifyWebhookSignatureMiddleware(config.Secret)(http.HandlerFunc(hookHandler)))

	address := config.Address + ":" + strconv.FormatInt(config.Port, 10)

	log.Println("Listening on " + address)

	//starting server
	err = http.ListenAndServe(address, nil)
	if err != nil {
		log.Println(err)
	}
}

func loadConfig(configFile string) Config {
	var file, err = os.Open(configFile)
	panicIf(err)

	// close file on exit and check for its returned error
	defer func() {
		panicIf(file.Close())
	}()

	buffer := make([]byte, 1024)

	count, err := file.Read(buffer)
	panicIf(err)

	err = json.Unmarshal(buffer[:count], &config)
	panicIf(err)

	return config
}

func hookHandler(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if reason := recover(); reason != nil {
			httpError(w, r, http.StatusBadRequest, fmt.Errorf("panic: %v", reason))
		}
	}()

	if r.Method != http.MethodPost {
		httpError(w, r, http.StatusMethodNotAllowed, nil)
		return
	}
	//get the hook event from the headers
	event := r.Header.Get("X-Gitea-Event")
	//only push events are current supported
	if event != "push" {
		httpError(w, r, http.StatusBadRequest, fmt.Errorf("unhandled event %v", event))
		return
	}
	//read request body
	var data, err = io.ReadAll(r.Body)
	if err != nil {
		httpError(w, r, http.StatusBadRequest, fmt.Errorf("reading body: %w", err))
		return
	}
	//unmarshal request body
	var hook api.PushPayload
	err = json.Unmarshal(data, &hook)
	panicIf(err, fmt.Sprintf("while unmarshaling request base64(%s)", b64.StdEncoding.EncodeToString(data)))

	log.Printf("received webhook on %s", hook.Repo.FullName)

	//find matching config for repository name
	for _, repo := range config.Repositories {

		if repo.Name == hook.Repo.FullName || repo.Name == hook.Repo.HTMLURL {
			//execute commands for repository
			for _, cmd := range repo.Commands {
				var command = exec.Command(cmd)
				command.Env = append(os.Environ(),
					fmt.Sprintf("REPO_NAME=%v", hook.Repo.FullName),
					fmt.Sprintf("REPO_OWNER=%v", hook.Repo.Owner.Email),
					fmt.Sprintf("REPO_REF=%v", hook.Ref),
					fmt.Sprintf("REPO_HEAD_COMMIT=%v", hook.HeadCommit.ID),
					fmt.Sprintf("REPO_HEAD_AUTHOR=%v", hook.HeadCommit.Author.Email),
				)
				out, err := command.CombinedOutput()
				if err != nil {
					log.Println(err)
					httpError(w, r, http.StatusInternalServerError, err)
				} else {
					log.Println("Executed: " + cmd)
					log.Println("Output: " + string(out))
					_, err = w.Write(out)
					if err != nil {
						log.Println(err)
					}
				}
			}
			break
		}
	}
}

func httpError(w http.ResponseWriter, r *http.Request, status int, err error) {
	http.Error(w, http.StatusText(status), status)
	if err != nil {
		log.Printf("%v:%v", status, err)
		fmt.Fprintf(w, "%v:%v", status, err)
	}
}
