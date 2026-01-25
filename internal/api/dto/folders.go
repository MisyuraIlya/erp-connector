package dto

// TODO

type ImageFoldersResponse struct {
	Folders []string `json:"folders"`
}

type ListFolderRequest struct {
	FolderPath string `json:"folderPath"`
}

type ListFolderResponse struct {
	FolderPath string   `json:"folderPath"`
	Files      []string `json:"files"`
}

type FileRequest struct {
	FolderPath string `json:"folderPath"`
}
