package cmd

import (
	"fmt"
	"os"

	"regexp"
	"strings"

	"github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/requestor"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var collaboratorCmd = &cobra.Command{
	Use:   "collaborator",
	Short: "Manage collaborators and access requests",
}

var emailRegex = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}$`)

func validateProjectAndUser(projectID, username string) error {
	if !emailRegex.MatchString(strings.ToLower(username)) {
		return fmt.Errorf("invalid username '%s': must be a valid email address", username)
	}

	parts := strings.Split(projectID, "-")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid project_id '%s': must be in the form 'program-project'", projectID)
	}

	return nil
}

func printRequest(r requestor.Request) {
	b, err := yaml.Marshal(r)
	if err != nil {
		fmt.Printf("ID: %s (Error formatting details: %v)\n", r.RequestID, err)
		return
	}
	fmt.Println(string(b))
}

func getRequestorClient() (requestor.RequestorInterface, func()) {
	if profile == "" {
		fmt.Println("Error: profile is required. Please specify a profile using the --profile flag.")
		os.Exit(1)
	}

	// Initialize logger
	logger, logCloser := logs.New(profile)

	// Initialize Gen3Interface handles selective initialization
	g3i, err := g3client.NewGen3Interface(profile, logger, g3client.WithClients(g3client.RequestorClient))
	if err != nil {
		fmt.Printf("Error accessing Gen3: %v\n", err)
		logCloser()
		os.Exit(1)
	}

	return g3i.Requestor(), logCloser
}

var collaboratorListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List requests",
	Run: func(cmd *cobra.Command, args []string) {
		mine, _ := cmd.Flags().GetBool("mine")
		active, _ := cmd.Flags().GetBool("active")
		username, _ := cmd.Flags().GetString("username")

		client, closer := getRequestorClient()
		defer closer()

		requests, err := client.ListRequests(cmd.Context(), mine, active, username)
		if err != nil {
			fmt.Printf("Error listing requests: %v\n", err)
			os.Exit(1)
		}

		for _, r := range requests {
			printRequest(r)
		}
	},
}

var collaboratorPendingCmd = &cobra.Command{
	Use:   "pending",
	Short: "List pending requests",
	Run: func(cmd *cobra.Command, args []string) {
		client, closer := getRequestorClient()
		defer closer()

		// Fetch all requests
		requests, err := client.ListRequests(cmd.Context(), false, false, "")
		if err != nil {
			fmt.Printf("Error listing requests: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Pending requests:")
		for _, r := range requests {
			if r.Status != "SIGNED" {
				printRequest(r)
			}
		}
	},
}

var collaboratorAddUserCmd = &cobra.Command{
	Use:   "add [project_id] [username]",
	Short: "Add a user to a project",
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(2)(cmd, args); err != nil {
			return err
		}
		return validateProjectAndUser(args[0], args[1])
	},
	Run: func(cmd *cobra.Command, args []string) {
		projectID := args[0]
		username := args[1]
		write, _ := cmd.Flags().GetBool("write")
		guppy, _ := cmd.Flags().GetBool("guppy")
		approve, _ := cmd.Flags().GetBool("approve")

		client, closer := getRequestorClient()
		defer closer()

		reqs, err := client.AddUser(cmd.Context(), projectID, username, write, guppy)
		if err != nil {
			fmt.Printf("Error adding user: %v\n", err)
			os.Exit(1)
		}

		if approve {
			fmt.Println("\nAuto-approving requests...")
			for _, r := range reqs {
				updatedReq, err := client.UpdateRequest(cmd.Context(), r.RequestID, "SIGNED")
				if err != nil {
					fmt.Printf("Error approving request %s: %v\n", r.RequestID, err)
				} else {
					fmt.Printf("Approved request %s:\n", updatedReq.RequestID)
					printRequest(*updatedReq)
				}
			}
		} else {
			fmt.Println("Created requests:")
			for _, r := range reqs {
				printRequest(r)
			}
			fmt.Printf("\nAn authorized user must approve these requests to add %s to %s\n", username, projectID)
		}
	},
}

var collaboratorRemoveUserCmd = &cobra.Command{
	Use:   "rm [project_id] [username]",
	Short: "Remove a user from a project",
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(2)(cmd, args); err != nil {
			return err
		}
		return validateProjectAndUser(args[0], args[1])
	},
	Run: func(cmd *cobra.Command, args []string) {
		projectID := args[0]
		username := args[1]
		approve, _ := cmd.Flags().GetBool("approve")

		client, closer := getRequestorClient()
		defer closer()

		reqs, err := client.RemoveUser(cmd.Context(), projectID, username)
		if err != nil {
			fmt.Printf("Error removing user: %v\n", err)
			os.Exit(1)
		}

		if approve {
			fmt.Println("\nAuto-approving revoke requests...")
			for _, r := range reqs {
				updatedReq, err := client.UpdateRequest(cmd.Context(), r.RequestID, "SIGNED")
				if err != nil {
					fmt.Printf("Error approving request %s: %v\n", r.RequestID, err)
				} else {
					fmt.Printf("Approved request %s:\n", updatedReq.RequestID)
					printRequest(*updatedReq)
				}
			}
		} else {
			fmt.Println("Created revoke requests:")
			for _, r := range reqs {
				printRequest(r)
			}
		}
	},
}

var collaboratorApproveCmd = &cobra.Command{
	Use:   "approve [request_id]",
	Short: "Approve a request (sign it)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		requestID := args[0]

		client, closer := getRequestorClient()
		defer closer()

		req, err := client.UpdateRequest(cmd.Context(), requestID, "SIGNED")
		if err != nil {
			fmt.Printf("Error approving request: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Approved request %s\n", req.RequestID)
		printRequest(*req)
	},
}

var collaboratorUpdateCmd = &cobra.Command{
	Use:    "update [request_id] [status]",
	Short:  "Update a request status",
	Hidden: true,
	Args:   cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		requestID := args[0]
		status := args[1]

		client, closer := getRequestorClient()
		defer closer()

		req, err := client.UpdateRequest(cmd.Context(), requestID, status)
		if err != nil {
			fmt.Printf("Error updating request: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Updated request %s to status %s\n", req.RequestID, req.Status)
	},
}

func init() {
	RootCmd.AddCommand(collaboratorCmd)
	collaboratorCmd.AddCommand(collaboratorListCmd)
	collaboratorCmd.AddCommand(collaboratorPendingCmd)
	collaboratorCmd.AddCommand(collaboratorAddUserCmd)
	collaboratorCmd.AddCommand(collaboratorRemoveUserCmd)
	collaboratorCmd.AddCommand(collaboratorApproveCmd)
	collaboratorCmd.AddCommand(collaboratorUpdateCmd)

	collaboratorListCmd.Flags().Bool("mine", false, "List my requests")
	collaboratorListCmd.Flags().Bool("active", false, "List only active requests")
	collaboratorListCmd.Flags().String("username", "", "List requests for user")

	collaboratorAddUserCmd.Flags().BoolP("write", "w", false, "Grant write access")
	collaboratorAddUserCmd.Flags().BoolP("guppy", "g", false, "Grant guppy admin access")
	collaboratorAddUserCmd.Flags().BoolP("approve", "a", false, "Automatically approve the requests")

	collaboratorRemoveUserCmd.Flags().BoolP("approve", "a", false, "Automatically approve the revoke requests")

	collaboratorCmd.PersistentFlags().StringVar(&profile, "profile", "", "Specify profile to use")
}
