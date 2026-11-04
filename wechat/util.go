package wechat

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	uuidlib "github.com/google/uuid"
	"golang.org/x/crypto/pbkdf2"
)

func getPathSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if !info.IsDir() {
		return info.Size()
	}
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

func uuid() string {
	newUUID, _ := uuidlib.NewRandom()
	return newUUID.String()
}

// ListFilesWithExtension extension eg: .wxapkg
func ListFilesWithExtension(dir, extension string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, extension) {
			paths = append(paths, path)
		}
		return nil
	})

	return paths, err
}

func decryptWxapkgFile(wxid string, dataByte []byte) ([]byte, error) {
	var (
		salt = "saltiest"
		iv   = "the iv: 16 bytes"
	)
	if len(dataByte) < 6+1024 {
		return nil, fmt.Errorf("加密文件长度 %d 不足 1030 字节", len(dataByte))
	}

	dk := pbkdf2.Key([]byte(wxid), []byte(salt), 1000, 32, sha1.New)
	block, err := aes.NewCipher(dk)
	if err != nil {
		return nil, err
	}
	blockMode := cipher.NewCBCDecrypter(block, []byte(iv))
	originData := make([]byte, 1024)
	blockMode.CryptBlocks(originData, dataByte[6:1024+6])

	afData := make([]byte, len(dataByte)-1024-6) // remove first 6 + 1024 byte
	var xorKey = byte(0x66)
	if len(wxid) >= 2 {
		xorKey = wxid[len(wxid)-2]
	}
	for i, b := range dataByte[1024+6:] { // from 6 + 1024 byte
		afData[i] = b ^ xorKey
	}

	originData = append(originData[:1023], afData...)

	return originData, nil
}

func ScanWxapkgItem(path string, scan bool) ([]WxapkgItem, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !stat.IsDir() || !scan {
		items := []WxapkgItem{{
			UUID:           uuid(),
			WxId:           "unknown",
			Location:       path,
			Size:           getPathSize(path),
			IsDir:          stat.IsDir(),
			LastModifyTime: stat.ModTime().Unix(),
		}}
		populateLocalAppNames(items)
		return items, nil
	}

	var result []WxapkgItem
	seen := make(map[string]struct{})
	for _, root := range packageRootsForScan(path) {
		entries, err := os.ReadDir(root)
		if err != nil {
			if isPermissionError(err) {
				return nil, &PermissionError{Path: root, Err: err}
			}
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dirName := entry.Name()
			localPath := filepath.Join(root, dirName)
			if !reWxId.MatchString(dirName) {
				if reWxId.MatchString(filepath.Base(root)) {
					dirName = filepath.Base(root)
					localPath = root
				} else {
					continue
				}
			}
			if _, ok := seen[localPath]; ok {
				continue
			}
			seen[localPath] = struct{}{}
			item := WxapkgItem{
				UUID:       uuid(),
				WxId:       dirName,
				Location:   localPath,
				EncryptKey: dirName,
				Size:       getPathSize(localPath),
				IsDir:      true,
			}
			if info, err := os.Stat(localPath); err == nil {
				item.LastModifyTime = info.ModTime().Unix()
			}
			result = append(result, item)
		}
		if reWxId.MatchString(filepath.Base(root)) {
			if _, ok := seen[root]; !ok {
				seen[root] = struct{}{}
				item := WxapkgItem{
					UUID:       uuid(),
					WxId:       filepath.Base(root),
					Location:   root,
					EncryptKey: filepath.Base(root),
					Size:       getPathSize(root),
					IsDir:      true,
				}
				if info, err := os.Stat(root); err == nil {
					item.LastModifyTime = info.ModTime().Unix()
				}
				result = append(result, item)
			}
		}
	}
	populateLocalAppNames(result)
	return result, nil
}
