package runner

import (
	"encoding/json"
	"fmt"
	"log"
	"lukso/apps/lukso-manager/downloader"
	"lukso/apps/lukso-manager/settings"
	"lukso/apps/lukso-manager/shared"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type startClientsRequestBody struct {
	Network  string
	Clients  []string
	Settings settings.Settings
}

type stopClientsRequestBody struct {
	Clients []string
}

type Commands struct {
	orchestrator *exec.Cmd
	pandora      *exec.Cmd
	vanguard     *exec.Cmd
	validator    *exec.Cmd
}

var CommandsByClient = Commands{}

func StartClients(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	decoder := json.NewDecoder(r.Body)
	var body startClientsRequestBody
	err := decoder.Decode(&body)
	if err != nil {
		shared.HandleError(err, w)
		return
	}

	network := body.Network

	oldConfig, oldConfigError := ReadConfig(network)
	if oldConfigError != nil {
		log.Println("Old config not available.")
	}

	dlError := downloader.DownloadConfigFiles(network)
	if dlError != nil {
		log.Println("Error when downloading config files for " + network)
		shared.HandleError(dlError, w)
		return
	}

	config, newConfigError := ReadConfig(network)
	if newConfigError != nil {
		log.Println("New config not available.")
		shared.HandleError(newConfigError, w)
		return
	}

	if oldConfig != nil {
		if oldConfig.GENESISTIME != config.GENESISTIME {
			err := os.RemoveAll(shared.NetworkDir + network + "/" + shared.DataDir)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	folder := shared.NetworkDir + body.Network + "/logs/" + fmt.Sprint(config.GENESISTIME)

	_, folderErr := os.Stat(folder)
	if folderErr != nil {
		os.MkdirAll(folder, 0775)
	}

	now := time.Now()
	timestamp := now.Unix()

	if shared.Contains(body.Clients, "vanguard") {
		vanCmd, errVanguard := startVanguard(body.Settings.Versions[settings.Vanguard], network, config, fmt.Sprint(timestamp))
		if errVanguard != nil {
			shared.HandleError(errVanguard, w)
			return
		}
		CommandsByClient.vanguard = vanCmd
	}

	if shared.Contains(body.Clients, "orchestrator") {
		orchCmd, errOrchestrator := startOrchestrator(body.Settings.Versions[settings.Orchestrator], network)
		if errOrchestrator != nil {
			shared.HandleError(errOrchestrator, w)
			return
		}
		CommandsByClient.orchestrator = orchCmd
	}

	if shared.Contains(body.Clients, "pandora") {
		version := body.Settings.Versions[settings.Pandora]
		cmdPandora, errPandora := startPandora(version, network, body.Settings, config, fmt.Sprint(timestamp))
		if errPandora != nil {
			shared.HandleError(errPandora, w)
			return
		}
		CommandsByClient.pandora = cmdPandora
	}

	if shared.Contains(body.Clients, "validator") {
		version := body.Settings.Versions[settings.Validator]
		cmdValidator, errValidator := startValidator(version, network, config, fmt.Sprint(timestamp))
		if errValidator != nil {
			shared.HandleError(errValidator, w)
			return
		}
		CommandsByClient.validator = cmdValidator
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode("Successfully started all the clients."); err != nil {
		panic(err)
	}
}

func StopClients(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	decoder := json.NewDecoder(r.Body)
	var body stopClientsRequestBody
	err := decoder.Decode(&body)
	if err != nil {
		shared.HandleError(err, w)
		return
	}

	if shared.Contains(body.Clients, "pandora") && CommandsByClient.pandora != nil {
		if err := CommandsByClient.pandora.Process.Kill(); err != nil {
			shared.HandleError(err, w)
			return
		}
	}

	if shared.Contains(body.Clients, "vanguard") {
		if CommandsByClient.vanguard.Process == nil {
			return
		}
		if err := CommandsByClient.vanguard.Process.Kill(); err != nil {
			shared.HandleError(err, w)
			return
		}
	}

	if shared.Contains(body.Clients, "orchestrator") {
		if err := CommandsByClient.orchestrator.Process.Kill(); err != nil {
			shared.HandleError(err, w)
			return
		}
	}

	if shared.Contains(body.Clients, "validator") {
		if err := CommandsByClient.validator.Process.Kill(); err != nil {
			shared.HandleError(err, w)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func StartBinary(client string, version string, args []string) (*exec.Cmd, error) {

	log.Println("STARTING " + client + "@" + version)
	log.Println("ARGS " + strings.Join(args, " "))
	command := exec.Command(shared.BinaryDir+client+"/"+version+"/"+client, args...)

	if startError := command.Start(); startError != nil {
		log.Println("ERROR STARTING " + client + "@" + version)
		log.Fatal(startError)
		return nil, startError
	}

	return command, nil

}
