// Copyright © 2019 Percona, LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package psmdb

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/Percona-Lab/percona-dbaas-cli/dbaas"
	"github.com/Percona-Lab/percona-dbaas-cli/dbaas/psmdb"
)

var delePVC *bool

// delCmd represents the list command
var delCmd = &cobra.Command{
	Use:   "delete-db <psmdb-cluster-name>",
	Short: "Delete MongoDB cluster",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return errors.New("You have to specify psmdb-cluster-name")
		}

		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		switch *deleteAnswerFormat {
		case "json":
			log.Formatter = new(logrus.JSONFormatter)
		}
		dbservice, err := dbaas.New(*envDlt)
		if err != nil {
			log.Errorln("new dbservice:", err.Error())
			return
		}
		sp := spinner.New(spinner.CharSets[14], 250*time.Millisecond)
		sp.Color("green", "bold")
		demo, err := cmd.Flags().GetBool("demo")
		if demo && err == nil {
			sp.UpdateCharSet([]string{""})
		}
		sp.Prefix = "Looking for the cluster..."
		sp.FinalMSG = ""
		sp.Start()
		defer sp.Stop()

		ext, err := dbservice.IsObjExists("psmdb", name)
		if err != nil {
			log.Errorln("check if cluster exists:", err.Error())
			return
		}

		if !ext {
			sp.Stop()
			log.Errorln("unable to find cluster psmdb/" + name)
			list, err := dbservice.List("psmdb")
			if err != nil {
				log.Errorln("psmdb cluster list:", err.Error())
				return
			}
			log.Errorln("avaliable clusters:", list)
			return
		}

		if *delePVC {
			sp.Stop()
			var yn string
			fmt.Printf("\nAll current data on \"%s\" cluster will be destroyed.\nAre you sure? [y/N] ", name)
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				yn = strings.TrimSpace(scanner.Text())
				break
			}
			if yn != "y" && yn != "Y" {
				return
			}
			sp.Start()
		}
		sp.Lock()
		sp.Prefix = "Deleting..."
		sp.Unlock()
		ok := make(chan string)
		cerr := make(chan error)

		go dbservice.Delete("psmdb", psmdb.New(name, "", defaultVersion, ""), *delePVC, ok, cerr)
		tckr := time.NewTicker(1 * time.Second)
		defer tckr.Stop()
		for {
			select {
			case <-ok:
				sp.FinalMSG = ""
				sp.Stop()
				log.Println("Deleting done")
				return
			case err := <-cerr:
				log.Errorln("delete psmdb:", err.Error())
				return
			}
		}
	},
}

var envDlt *string
var deleteAnswerFormat *string

func init() {
	delePVC = delCmd.Flags().Bool("clear-data", false, "Remove cluster volumes")
	envDlt = delCmd.Flags().String("environment", "", "Target kubernetes cluster")

	deleteAnswerFormat = delCmd.Flags().String("output", "", "Answers format")

	PSMDBCmd.AddCommand(delCmd)
}
