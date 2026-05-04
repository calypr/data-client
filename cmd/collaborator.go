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

var collaboratorsCmd = &cobra.Command{
	Use:   "collaborators",
	Short: "Manage collaborators and access requests",
}

var emailRegex = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}$`)

func printRequest(r requestor.Request) {
	b, err := yaml.Marshal(r)
	if err != nil {
		fmt.Printf("ID: %s (Error formatting details: %v)\n", r.RequestID, err)
		return
	}
	fmt.Println(string(b))
}

func getRequestorClient(localProfile string) (requestor.RequestorInterface, func()) {
	if localProfile == "" {
		fmt.Println("Error: profile is required.")
		os.Exit(1)
	}

	// Initialize logger
	logger, logCloser := logs.New(localProfile)

	// Initialize base Gen3 interface and build requestor client from it.
	g3i, err := g3client.NewGen3Interface(localProfile, logger)
	if err != nil {
		fmt.Printf("Error accessing Gen3: %v\n", err)
		logCloser()
		os.Exit(1)
	}

	return requestor.NewRequestorClient(g3i, g3i.Credentials().Current()), logCloser
}

var collaboratorListCmd = &cobra.Command{
	Use:   "ls [profile]",
	Short: "List requests",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		p := args[0]
		mine, _ := cmd.Flags().GetBool("mine")
		active, _ := cmd.Flags().GetBool("active")
		username, _ := cmd.Flags().GetString("username")

		client, closer := getRequestorClient(p)
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
	Use:   "pending [profile]",
	Short: "List pending requests",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		p := args[0]
		client, closer := getRequestorClient(p)
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
	Use:   "add [profile] [email] [program] [project]",
	Short: "Add a user to a project",
	Args:  cobra.ExactArgs(4),
	Run: func(cmd *cobra.Command, args []string) {
		p := args[0]
		username := args[1]
		program := args[2]
		project := args[3]
		projectID := fmt.Sprintf("%s-%s", program, project)

		if !emailRegex.MatchString(strings.ToLower(username)) {
			fmt.Printf("Error: invalid email address '%s'\n", username)
			os.Exit(1)
		}

		write, _ := cmd.Flags().GetBool("write")
		guppy, _ := cmd.Flags().GetBool("guppy")
		approve, _ := cmd.Flags().GetBool("approve")

		client, closer := getRequestorClient(p)
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

var collaboratorBulkAddUserCmd = &cobra.Command{
	Use:   "bulk-add [profile] [email] [resource_paths...]",
	Short: "Add a user to multiple project resources",
	Long:  "Add a user to multiple project resources from a comma-delimited list. Resource paths may be /programs/<organization>/projects/<project> or organization/project.",
	Args:  cobra.MinimumNArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		p := args[0]
		username := args[1]
		resourceList := strings.Join(args[2:], " ")

		if !emailRegex.MatchString(strings.ToLower(username)) {
			fmt.Printf("Error: invalid email address '%s'\n", username)
			os.Exit(1)
		}

		resources, err := requestor.ParseProjectResources(resourceList)
		if err != nil {
			fmt.Printf("Error parsing resource paths: %v\n", err)
			os.Exit(1)
		}

		write, _ := cmd.Flags().GetBool("write")
		guppy, _ := cmd.Flags().GetBool("guppy")
		approve, _ := cmd.Flags().GetBool("approve")

		client, closer := getRequestorClient(p)
		defer closer()

		fmt.Printf("Creating collaborator requests for %d project resources...\n", len(resources))
		reqs, err := client.AddUserToResources(cmd.Context(), resources, username, write, guppy)
		if err != nil {
			fmt.Printf("Error adding user to resources: %v\n", err)
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
			fmt.Printf("\nAn authorized user must approve these requests to add %s to %d project resources\n", username, len(resources))
		}
	},
}

var collaboratorRemoveUserCmd = &cobra.Command{
	Use:   "rm [profile] [email] [program] [project]",
	Short: "Remove a user from a project",
	Args:  cobra.ExactArgs(4),
	Run: func(cmd *cobra.Command, args []string) {
		p := args[0]
		username := args[1]
		program := args[2]
		project := args[3]
		projectID := fmt.Sprintf("%s-%s", program, project)

		if !emailRegex.MatchString(strings.ToLower(username)) {
			fmt.Printf("Error: invalid email address '%s'\n", username)
			os.Exit(1)
		}

		approve, _ := cmd.Flags().GetBool("approve")

		client, closer := getRequestorClient(p)
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
	Use:   "approve [profile] [request_id]",
	Short: "Approve a request (sign it)",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		p := args[0]
		requestID := args[1]

		client, closer := getRequestorClient(p)
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
	Use:    "update [profile] [request_id] [status]",
	Short:  "Update a request status",
	Hidden: true,
	Args:   cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		p := args[0]
		requestID := args[1]
		status := args[2]

		client, closer := getRequestorClient(p)
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
	RootCmd.AddCommand(collaboratorsCmd)
	collaboratorsCmd.AddCommand(collaboratorListCmd)
	collaboratorsCmd.AddCommand(collaboratorPendingCmd)
	collaboratorsCmd.AddCommand(collaboratorAddUserCmd)
	collaboratorsCmd.AddCommand(collaboratorBulkAddUserCmd)
	collaboratorsCmd.AddCommand(collaboratorRemoveUserCmd)
	collaboratorsCmd.AddCommand(collaboratorApproveCmd)
	collaboratorsCmd.AddCommand(collaboratorUpdateCmd)

	collaboratorListCmd.Flags().Bool("mine", false, "List my requests")
	collaboratorListCmd.Flags().Bool("active", false, "List only active requests")
	collaboratorListCmd.Flags().String("username", "", "List requests for user")

	collaboratorAddUserCmd.Flags().BoolP("write", "w", false, "Grant write access")
	collaboratorAddUserCmd.Flags().BoolP("guppy", "g", false, "Grant guppy admin access")
	collaboratorAddUserCmd.Flags().BoolP("approve", "a", false, "Automatically approve the requests")

	collaboratorBulkAddUserCmd.Flags().BoolP("write", "w", false, "Grant write access")
	collaboratorBulkAddUserCmd.Flags().BoolP("guppy", "g", false, "Grant guppy admin access")
	collaboratorBulkAddUserCmd.Flags().BoolP("approve", "a", false, "Automatically approve the requests")

	collaboratorRemoveUserCmd.Flags().BoolP("approve", "a", false, "Automatically approve the revoke requests")
}
