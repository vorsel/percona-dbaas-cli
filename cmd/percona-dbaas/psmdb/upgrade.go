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
	"github.com/spf13/cobra"

	"github.com/Percona-Lab/percona-dbaas-cli/dbaas"
	"github.com/Percona-Lab/percona-dbaas-cli/dbaas/psmdb"
)

// upgradeCmd represents the edit command
var upgradeCmd = &cobra.Command{
	Use:   "upgrade <psmdb-cluster-name> <to-version>",
	Short: "Upgrade Percona Server MongoDB cluster",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return errors.New("You have to specify psmdb-cluster-name")
		}

		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		app := psmdb.New(name, "doesnotMatter", defaultVersion)

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

		ext, err := dbaas.IsObjExists("psmdb", name)

		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] check if cluster exists: %v\n", err)
			return
		}

		if !ext {
			sp.Stop()
			fmt.Fprintf(os.Stderr, "Unable to find cluster \"%s/%s\"\n", "psmdb", name)
			list, err := dbaas.List("psmdb")
			if err != nil {
				return
			}
			fmt.Println("Avaliable clusters:")
			fmt.Print(list)
			return
		}

		created := make(chan string)
		msg := make(chan dbaas.OutuputMsg)
		cerr := make(chan error)

		oparg := ""
		if len(args) > 1 {
			oparg = args[1]
		}
		operator, appsImg, err := app.Images(oparg, cmd.Flags())
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] setup images for upgrade: %v\n", err)
			return
		}

		if operator != "" {
			num, err := dbaas.Instances("psmdb")
			if err != nil {
				fmt.Fprintf(os.Stderr, "[ERROR] unable to get psmdb instances: %v\n", err)
				return
			}
			if len(num) > 1 {
				sp.Stop()
				var yn string
				fmt.Printf("\nFound more than one psmdb cluster: %v.\nOperator upgrade may affect other clusters.\nContinue? [y/N] ", num)
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
		}

		go dbaas.Upgrade("psmdb", app, operator, appsImg, created, msg, cerr)
		sp.Prefix = "Upgrading cluster..."

		for {
			select {
			case <-created:
				okmsg, _ := dbaas.ListName("psmdb", name)
				sp.FinalMSG = fmt.Sprintf("Upgrading cluster...[done]\n\n%s", okmsg)
				return
			case omsg := <-msg:
				switch omsg.(type) {
				case dbaas.OutuputMsgDebug:
					// fmt.Printf("\n[debug] %s\n", omsg)
				case dbaas.OutuputMsgError:
					sp.Stop()
					fmt.Printf("[operator log error] %s\n", omsg)
					sp.Start()
				}
			case err := <-cerr:
				fmt.Fprintf(os.Stderr, "\n[ERROR] upgrade psmdb: %v\n", err)
				sp.HideCursor = true
				return
			}
		}
	},
}

func init() {
	upgradeCmd.Flags().String("operator-image", "", "Custom image to upgrade operator to")
	upgradeCmd.Flags().String("psmdb-image", "", "Custom image to upgrade psmdb to")
	upgradeCmd.Flags().String("backup-image", "", "Custom image to upgrade backup to")

	PSMDBCmd.AddCommand(upgradeCmd)
}
