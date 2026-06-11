package sower

type StatusResp struct {
	Uid    string `json:"uid"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type OutputResp struct {
	Output string `json:"output"`
}

type File struct {
	FileTitle string `json:"fileTitle,omitempty"`
	FilePath  string `json:"filePath"`
}

type DispatchArgs struct {
	Method         string `json:"method"`
	ProjectId      string `json:"projectId"`
	Profile        string `json:"profile"`
	BucketName     string `json:"bucketName"`
	APIEndpoint    string `json:"APIEndpoint"`
	GHCommitHash   string `json:"ghCommitHash"`
	GHPAccessToken string `json:"ghToken"`
	GHUserName     string `json:"ghUserName"`
	GHRepoURL      string `json:"ghRepoUrl"`
}

type JobArgs struct {
	Input  DispatchArgs `json:"input"`
	Action string       `json:"action"`
}
