package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// elasticCmd represents the elastic command
var elasticCmd = &cobra.Command{
	Use:   "elastic",
	Short: "send log data to elasticsearch",
	Long:  `Send log data to ElasticSearch`,
	Run: func(cmd *cobra.Command, args []string) {
		// TODO: Work your own magic here
		fmt.Println("elastic called")
	},
}

func init() {
	RootCmd.AddCommand(elasticCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// elasticCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// elasticCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

}
