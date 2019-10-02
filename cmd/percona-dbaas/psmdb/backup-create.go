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
	"time"

	"github.com/briandowns/spinner"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/Percona-Lab/percona-dbaas-cli/dbaas"
	"github.com/Percona-Lab/percona-dbaas-cli/dbaas/psmdb"
)

// bcpCmd represents the list command
var bcpCmd = &cobra.Command{
	Use:   "create-backup <psmdb-cluster-name>",
	Short: "Create MongoDB backup",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return errors.New("You have to specify psmdb-cluster-name")
		}

		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		switch *backupCreateAnswerFormat {
		case "json":
			log.Formatter = new(logrus.JSONFormatter)
		}
		dbservice, err := dbaas.New(*envBckpCrt)
		if err != nil {
			log.Errorln("new dbservice:", err.Error())
			return
		}
		sp := spinner.New(spinner.CharSets[14], 250*time.Millisecond)
		sp.Color("green", "bold")
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
				log.Errorln("psmdb clusters list:", err.Error())
				return
			}
			log.Println("avaliable clusters:", list)
			return
		}
		sp.Lock()
		sp.Prefix = "Creating backup..."
		sp.Unlock()
		bcp := psmdb.NewBackup(name)

		bcp.Setup(dbaas.DefaultBcpStorageName)

		ok := make(chan string)
		msg := make(chan dbaas.OutuputMsg)
		cerr := make(chan error)

		go dbservice.ApplyCheck("psmdb-backup", bcp, ok, msg, cerr)
		tckr := time.NewTicker(1 * time.Second)
		defer tckr.Stop()
		for {
			select {
			case okmsg := <-ok:
				sp.FinalMSG = ""
				sp.Stop()
				log.Println("Creating backup done.", okmsg)
				return
			case omsg := <-msg:
				switch omsg.(type) {
				case dbaas.OutuputMsgDebug:
					// fmt.Printf("\n[debug] %s\n", omsg)
				case dbaas.OutuputMsgError:
					sp.Stop()
					log.Errorln("operator log error:", omsg.String())
					sp.Start()
				}
			case err := <-cerr:
				log.Errorln("create backup:", err.Error())
				return
			}
		}
	},
}

var envBckpCrt *string
var backupCreateAnswerFormat *string

func init() {
	envBckpCrt = bcpCmd.Flags().String("environment", "", "Target kubernetes cluster")
	backupCreateAnswerFormat = bcpCmd.Flags().String("output", "", "Answers format")
	PSMDBCmd.AddCommand(bcpCmd)
}
