package cmd

import (
	"fmt"

	"github.com/morikuni/aec"
	"github.com/spf13/cobra"
)

const WelcomeMessage = "FaasEdge Core (fecore)"

func init() {
	rootCommand.AddCommand(versionCmd)
	rootCommand.AddCommand(upCmd)
	rootCommand.AddCommand(installCmd)
	rootCommand.AddCommand(makeProviderCmd())
	rootCommand.AddCommand(collectCmd)
}

func RootCommand() *cobra.Command {
	return rootCommand
}

var (
	// GitCommit Git Commit SHA
	GitCommit string
	// Version version of the CLI
	Version string
)

// Execute fecore
func Execute(version, gitCommit string) error {

	// Get Version and GitCommit values from main.go.
	Version = version
	GitCommit = gitCommit

	if err := rootCommand.Execute(); err != nil {
		return err
	}
	return nil
}

var rootCommand = &cobra.Command{
	Use:   "fecore",
	Short: "Start fecore",
	Long: `
faasedge-core
`,
	RunE:         runRootCommand,
	SilenceUsage: true,
}

func runRootCommand(cmd *cobra.Command, args []string) error {

	printLogo()
	cmd.Help()

	return nil
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display version information.",
	Run:   parseBaseCommand,
}

func parseBaseCommand(_ *cobra.Command, _ []string) {
	printLogo()

	printVersion()
}

func printVersion() {
	fmt.Printf("fecore version: %s\tcommit: %s\n", GetVersion(), GitCommit)
}

func printLogo() {
	logoText := aec.WhiteF.Apply(Logo)
	fmt.Println(logoText)
}

// GetVersion get latest version
func GetVersion() string {
	if len(Version) == 0 {
		return "dev"
	}
	return Version
}

// Logo for version and root command
const Logo = `
 _____                        _                   ____               
|  ___|_ _  __ _ ___  ___  __| | __ _  ___       / ___|___  _ __ ___ 
| |_ / _ |/ _  / __|/ __ \/ _ |  / _ |/ _ \_____|  /  /   \|__ _/ _ \
|  _| (_| | (_| \__ \  __/ (_| | (_| |  __/_____| |__| (_) | | |  __/
|_|  \__,_|\__,_|___/\___|\__,_|\__, |\___|      \____\___/|_|  \___|
                                |___/                                
`
