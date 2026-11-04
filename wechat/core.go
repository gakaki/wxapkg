package wechat

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

var reWxId = regexp.MustCompile(`^wx[0-9a-f]{16}$`)

type StatusType string

var (
	StatusTypeRunning  StatusType = "running"
	StatusTypeFinished StatusType = "finished"
	StatusTypeError    StatusType = "error"
)

type UnpackOptions struct {
	EnableDecrypt      bool
	EnableJsBeautify   bool
	EnableHtmlBeautify bool
	EnableJsonBeautify bool
	OutputDir          string
	SavePath           string
}

type WxapkgItem struct {
	UUID                   string
	WxId                   string
	Name                   string
	NameProvider           string
	NameProviderID         string
	FallbackNameProvider   string
	FallbackNameProviderID string
	Location               string
	EncryptKey             string
	Size                   int64
	IsDir                  bool
	LastModifyTime         int64
	WxapkgFilePaths        []string

	UnpackStatus       StatusType
	UnpackCurrent      int64
	UnpackTotal        int64
	UnpackProgress     float64
	UnpackCurrentFile  string
	UnpackSavePath     string
	UnpackErrorMessage string
}

func (u *WxapkgItem) SetErrorState(msg string) {
	u.UnpackErrorMessage = msg
	u.UnpackStatus = StatusTypeError
}

func (u *WxapkgItem) IncreaseProgress(currentFile string) {
	u.UnpackCurrent++
	u.UnpackCurrentFile = currentFile
	if u.UnpackTotal != 0 {
		u.UnpackProgress = float64(u.UnpackCurrent) / float64(u.UnpackTotal) * 100
	}
}

type WxapkgFileItemStructure struct {
	name     string
	offset   uint32
	size     uint32
	savePath string

	rawFileData *[]byte
	rawFilePath *string
}

// cleanWxapkgPath converts an index entry to a path relative to the package
// root. Recent WeChat packages use entries such as "/static/app.js", where
// the leading slash denotes the package root rather than the host filesystem.
func cleanWxapkgPath(rawName string) (string, error) {
	name := strings.ReplaceAll(strings.TrimSpace(rawName), `\`, "/")
	name = strings.TrimLeft(name, "/")
	if name == "" || strings.ContainsRune(name, 0) {
		return "", errors.New("文件名无效")
	}

	cleanName := path.Clean(name)
	if cleanName == "." || cleanName == ".." || strings.HasPrefix(cleanName, "../") {
		return "", errors.Errorf("文件名 %s 会导致目录穿越", rawName)
	}
	if len(cleanName) >= 2 && cleanName[1] == ':' &&
		((cleanName[0] >= 'a' && cleanName[0] <= 'z') || (cleanName[0] >= 'A' && cleanName[0] <= 'Z')) {
		return "", errors.Errorf("文件名 %s 是主机绝对路径", rawName)
	}
	return cleanName, nil
}

// PathScanResult GetDefaultPaths 的返回类型
type PathScanResult struct {
	Paths                 []string // 检测到的小程序安装目录
	Logs                  string   // 检测过程的日志详情
	NeedsFullDiskAccess   bool     // macOS 阻止访问微信沙盒目录
	PermissionBlockedPath string   // 被系统拒绝访问的路径
}

type platform interface {
	GetDefaultPaths() PathScanResult
}

var Platform = newPlatform()

type Unpacker struct {
	item    *WxapkgItem
	options *UnpackOptions
	locker  sync.Mutex

	files       []*WxapkgFileItemStructure
	fileNameMap map[string]bool
}

func NewUnpacker(item *WxapkgItem, options *UnpackOptions) *Unpacker {
	return &Unpacker{
		item:        item,
		options:     options,
		locker:      sync.Mutex{},
		files:       []*WxapkgFileItemStructure{},
		fileNameMap: map[string]bool{},
	}
}

func (u *Unpacker) init() error {
	var err error
	u.options.OutputDir, err = filepath.Abs(u.options.OutputDir)
	if err != nil {
		return errors.Errorf("检查输出目录 '%s' 出错，%v", u.options.OutputDir, err)
	}

	if u.item.IsDir {
		files, err := ListFilesWithExtension(u.item.Location, ".wxapkg")
		if err != nil {
			return errors.Errorf("扫描目录 '%s' 下 wxapkg 文件失败，%v", u.item.Location, err)
		}
		if len(files) == 0 {
			return errors.Errorf("目录 '%s' 下没有 wxapkg 文件", u.item.Location)
		}
		u.item.WxapkgFilePaths = files
	} else {
		u.item.WxapkgFilePaths = []string{u.item.Location}
	}

	// 直接使用前端传入的SavePath
	u.item.UnpackSavePath = u.options.SavePath

	return nil
}

func (u *Unpacker) analyzeAll() error {
	for _, wxapkgFile := range u.item.WxapkgFilePaths {
		fileData, err := os.ReadFile(wxapkgFile)
		if err != nil {
			return err
		}

		if !u.isDecryptedWxapkgFile(fileData) {
			if !u.options.EnableDecrypt {
				return errors.Errorf("小程序文件 '%s' 为加密文件，请在解包配置中启用解密", wxapkgFile)
			}
			if u.item.EncryptKey == "" {
				return errors.Errorf("小程序文件 '%s' 为加密文件，未设置解密密钥，秘钥为小程序的 wxid，格式：^wx[0-9a-f]{16}$", wxapkgFile)
			}

			fileData, err = decryptWxapkgFile(u.item.EncryptKey, fileData)
			if err != nil {
				return errors.Errorf("解密小程序文件 '%s' 失败, %v", wxapkgFile, err)
			}
		}

		files, err := u.analyze(fileData, wxapkgFile)
		if err != nil {
			return errors.Errorf("解析小程序文件 '%s' 失败, %v", wxapkgFile, err)
		}

		u.files = append(u.files, files...)
	}

	return nil
}

func (u *Unpacker) isDecryptedWxapkgFile(data []byte) bool {
	if len(data) < 13 {
		return false
	}

	return data[0] == 0xBE && data[13] == 0xED // the firstMark and lastMark
}

func (u *Unpacker) analyze(data []byte, wxapkgFilePath string) ([]*WxapkgFileItemStructure, error) {
	var f = bytes.NewReader(data)

	// Read header
	var (
		firstMark       uint8
		info1           uint32
		indexInfoLength uint32
		bodyInfoLength  uint32
		lastMark        uint8
	)
	if err := binary.Read(f, binary.BigEndian, &firstMark); err != nil {
		return nil, errors.Wrap(err, "读取文件头失败")
	}
	if err := binary.Read(f, binary.BigEndian, &info1); err != nil {
		return nil, errors.Wrap(err, "读取文件头失败")
	}
	if err := binary.Read(f, binary.BigEndian, &indexInfoLength); err != nil {
		return nil, errors.Wrap(err, "读取索引长度失败")
	}
	if err := binary.Read(f, binary.BigEndian, &bodyInfoLength); err != nil {
		return nil, errors.Wrap(err, "读取正文长度失败")
	}
	if err := binary.Read(f, binary.BigEndian, &lastMark); err != nil {
		return nil, errors.Wrap(err, "读取文件尾失败")
	}

	if firstMark != 0xBE || lastMark != 0xED {
		return nil, errors.New("wxapkg 文件结构不合法")
	}

	var fileCount uint32
	if err := binary.Read(f, binary.BigEndian, &fileCount); err != nil {
		return nil, errors.Wrap(err, "读取文件数量失败")
	}
	if fileCount > 102400 {
		return nil, errors.Errorf("文件总数量 %d 超出上限 102400", fileCount)
	}

	// Read index
	var result = make([]*WxapkgFileItemStructure, fileCount)
	for i := uint32(0); i < fileCount; i++ {
		var nameLen = uint32(0)
		if err := binary.Read(f, binary.BigEndian, &nameLen); err != nil {
			return nil, errors.Wrap(err, "读取文件名长度失败")
		}

		if nameLen > 1024 {
			return nil, errors.Errorf("文件名长度 %d 超出上限 1024 字节", nameLen)
		}

		var nameBytes = make([]byte, nameLen)
		if _, err := io.ReadFull(f, nameBytes); err != nil {
			return nil, errors.Wrap(err, "读取文件名失败")
		}

		var item = &WxapkgFileItemStructure{
			rawFileData: &data,
			rawFilePath: &wxapkgFilePath,
		}
		if err := binary.Read(f, binary.BigEndian, &item.offset); err != nil {
			return nil, errors.Wrap(err, "读取文件偏移失败")
		}
		if err := binary.Read(f, binary.BigEndian, &item.size); err != nil {
			return nil, errors.Wrap(err, "读取文件长度失败")
		}

		// Recent packages may prefix entries with a slash. Use a package-root
		// relative path for deduplication and extraction.
		cleanName, err := cleanWxapkgPath(string(nameBytes))
		if err != nil {
			return nil, err
		}
		item.name = cleanName

		// check if a file path exists, use a.txt -> a-1.txt -> a-2.txt
		var j = 1
		for u.fileNameMap[item.name] {
			dot := strings.LastIndex(item.name, ".")
			if dot == -1 { // no extension
				item.name = fmt.Sprintf("%s-%d", cleanName, j)
			} else {
				item.name = fmt.Sprintf("%s-%d%s", cleanName[:dot], j, cleanName[dot:])
			}
			j++
		}
		u.fileNameMap[item.name] = true

		root, err := filepath.Abs(u.item.UnpackSavePath)
		if err != nil {
			return nil, errors.Errorf("输出目录 '%s' 解析失败，%v", u.item.UnpackSavePath, err)
		}
		item.savePath, err = filepath.Abs(filepath.Join(root, filepath.FromSlash(item.name)))
		if err != nil {
			return nil, errors.Errorf("文件名拼接出错，%v", err)
		}
		rel, err := filepath.Rel(root, item.savePath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return nil, errors.Errorf("文件名 %s 会导致目录穿越", item.name)
		}
		if item.size > 10*1024*1024 {
			return nil, errors.Errorf("文件名 %s 标记长度 %d 超出上限 10 MB", item.name, item.size)
		}
		if uint64(item.offset)+uint64(item.size) > uint64(len(data)) {
			return nil, errors.Errorf("文件名 %s 的数据范围超出文件大小", item.name)
		}

		result[i] = item
	}

	return result, nil
}

func (u *Unpacker) UnpackWithStatusCallback(callback func(item *WxapkgItem)) {
	if err := u.init(); err != nil {
		u.item.SetErrorState(err.Error())
		callback(u.item)
		return
	}

	if err := u.analyzeAll(); err != nil {
		u.item.SetErrorState(err.Error())
		callback(u.item)
		return
	}

	u.item.UnpackTotal += int64(len(u.files))
	u.item.UnpackStatus = StatusTypeRunning
	callback(u.item)

	var hasError = u.unpack(20, callback)

	if !hasError {
		u.item.UnpackStatus = StatusTypeFinished
		callback(u.item)
	}
}

func (u *Unpacker) lock(f func()) {
	u.locker.Lock()
	defer u.locker.Unlock()
	f()
}

func (u *Unpacker) unpack(thread int, callback func(item *WxapkgItem)) bool {
	// Save files
	var chFiles = make(chan *WxapkgFileItemStructure)
	var wg = sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()

		for _, d := range u.files {
			chFiles <- d
		}

		close(chFiles)
	}()

	var hasError = false

	wg.Add(thread)
	for i := 0; i < thread; i++ {
		go func() {
			defer wg.Done()

			for d := range chFiles {
				u.locker.Lock()
				if hasError {
					return
				}
				u.locker.Unlock()

				dir := filepath.Dir(d.savePath)
				err := os.MkdirAll(dir, os.ModePerm)
				if err != nil {
					u.lock(func() {
						u.item.SetErrorState(fmt.Sprintf("解包小程序文件 %s 时出错，创建目录 %s 失败，%v", *d.rawFilePath, dir, err))
						if !hasError {
							callback(u.item)
						}
						hasError = true
					})
					return
				}

				data := (*d.rawFileData)[d.offset : d.offset+d.size]

				var ext = filepath.Ext(d.name)
				if ext == ".json" && u.options.EnableJsonBeautify {
					data = PrettyJson(data)
				} else if ext == ".html" && u.options.EnableHtmlBeautify {
					data = PrettyHtml(data)
				} else if ext == ".js" && u.options.EnableJsBeautify {
					data = PrettyJavaScript(data)
				}

				err = os.WriteFile(d.savePath, data, 0600)
				if err != nil {
					u.lock(func() {
						u.item.SetErrorState(fmt.Sprintf("解包小程序文件 %s 时出错，写入文件 %s 失败，%v", *d.rawFilePath, d.savePath, err))
						if !hasError {
							callback(u.item)
						}
						hasError = true
					})
					return
				}

				u.lock(func() {
					u.item.IncreaseProgress(d.name)
					if !hasError {
						callback(u.item)
					}
				})
			}
		}()
	}

	wg.Wait()

	return hasError
}
