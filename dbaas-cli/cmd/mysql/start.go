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

package mysql

import (
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/Percona-Lab/percona-dbaas-cli/dbaas-cli/pb"
	dbaas "github.com/Percona-Lab/percona-dbaas-cli/dbaas-lib"
	_ "github.com/Percona-Lab/percona-dbaas-cli/dbaas-lib/engines/k8s-pxc"
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start-db <mysql-cluster-name>",
	Short: "Start MySQL cluster ",
	Long:  "Start MySQL cluster that have been stopped before.",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return errors.New("You have to specify resource name")
		}

		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		output, err := cmd.Flags().GetString("output")
		if err != nil {
			log.Error("get output flag: ", err)
		}

		var dotPrinter pb.ProgressBar
		switch output {
		case "json":
			dotPrinter = pb.NewNoOp()
		default:
			dotPrinter = pb.NewDotPrinter()
		}

		noWait, err := cmd.Flags().GetBool("no-wait")
		if err != nil {
			log.Error("get no-wait flag: ", err)
		}

		startOptions := addSpec("pause=false")

		instance := dbaas.Instance{
			Name:          args[0],
			EngineOptions: startOptions,
			Engine:        *startEngine,
			Provider:      *startProvider,
		}
		dotPrinter.Start("Starting")
		err = dbaas.ModifyDB(instance)
		if err != nil {
			dotPrinter.Stop("error")
			log.Error("modify db: ", err)
			return
		}
		time.Sleep(time.Second * 6) //let k8s time for applying new cr
		tries := 0
		tckr := time.NewTicker(500 * time.Millisecond)
		defer tckr.Stop()
		for range tckr.C {
			cluster, err := dbaas.DescribeDB(instance)
			if err != nil {
				//dotPrinter.StopPrintDot("error")
				//log.Error("check db: ", err)
				continue
			}
			cluster.Pass = ""
			switch cluster.Status {
			case "ready":
				dotPrinter.Stop("done")
				log.WithField("database", cluster).Info("Database started successfully, connection details are below:")
				return
			case "initializing":
				if noWait {
					log.WithField("database", cluster).Info("information")
					return
				}
			}
			if tries >= maxTries {
				dotPrinter.Stop("error")
				log.Error("unable to modify cluster. cluster status: ", cluster.Status)

				return
			}
			tries++
		}
	},
}

var startProvider *string
var startEngine *string

func init() {
	startProvider = startCmd.Flags().String("provider", "k8s", "Provider")
	startEngine = startCmd.Flags().String("engine", "pxc", "Engine")

	PXCCmd.AddCommand(startCmd)
}
