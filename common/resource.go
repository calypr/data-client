package common

import (
	"fmt"
	"strings"
)

func ProjectToResource(project string) (string, error) {
	if project == "" {
		return "", fmt.Errorf("error: project ID is empty")
	}
	if !strings.Contains(project, "-") {
		return "/programs/default/projects/" + project, nil
	}
	projectIdArr := strings.SplitN(project, "-", 2)
	return "/programs/" + projectIdArr[0] + "/projects/" + projectIdArr[1], nil
}
