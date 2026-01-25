package handlers

import (
	"net/http"

	"erp-connector/internal/api/dto"
	"erp-connector/internal/api/utils"
	"erp-connector/internal/files"
)

func NewListImageFoldersHandler(imageFolders []string) http.HandlerFunc {
	resp := dto.ImageFoldersResponse{Folders: append([]string(nil), imageFolders...)}
	return func(w http.ResponseWriter, r *http.Request) {
		utils.WriteJSON(w, http.StatusOK, resp)
	}
}

func NewListFolderFilesHandler(imageFolders []string) http.HandlerFunc {
	allowed, cfgErr := files.BuildAllowedFolders(imageFolders)
	return func(w http.ResponseWriter, r *http.Request) {
		if cfgErr != nil {
			utils.WriteError(w, http.StatusInternalServerError, "Folder configuration invalid", "FOLDER_CONFIG_INVALID", nil)
			return
		}

		folders := make([]dto.FolderFiles, 0, len(allowed))
		for _, item := range allowed {
			filesList, err := files.ListFiles(item.Canonical)
			if err != nil {
				utils.WriteError(w, http.StatusInternalServerError, "Failed to list folder", "FOLDER_LIST_FAILED", map[string]any{
					"folderPath": item.Original,
				})
				return
			}
			folders = append(folders, dto.FolderFiles{
				FolderPath: item.Original,
				Files:      filesList,
			})
		}

		utils.WriteJSON(w, http.StatusOK, dto.ListFoldersResponse{Folders: folders})
	}
}
