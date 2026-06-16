package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	appTitle = "Video2Text"

	cwUseDefault = int32(-2147483648)

	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	wsChild            = 0x40000000
	wsTabStop          = 0x00010000
	wsBorder           = 0x00800000
	wsVScroll          = 0x00200000
	wsClipChildren     = 0x02000000

	esMultiline   = 0x0004
	esAutovscroll = 0x0040
	esReadonly    = 0x0800
	esAutoHScroll = 0x0080

	bsPushButton = 0x00000000
	bsOwnerDraw  = 0x0000000B

	wmCreate         = 0x0001
	wmDestroy        = 0x0002
	wmPaint          = 0x000F
	wmDrawItem       = 0x002B
	wmCommand        = 0x0111
	wmClose          = 0x0010
	wmEraseBkgnd     = 0x0014
	wmCtlColorEdit   = 0x0133
	wmCtlColorStatic = 0x0138
	wmApp            = 0x8000

	msgLog      = wmApp + 1
	msgProgress = wmApp + 2
	msgDone     = wmApp + 3

	swShow = 5

	ofnFileMustExist = 0x00001000
	ofnPathMustExist = 0x00000800
	ofnExplorer      = 0x00080000
	ofnNoChangeDir   = 0x00000008

	pbmSetRange   = 0x0401
	pbmSetPos     = 0x0402
	emSetSel      = 0x00B1
	emScrollCaret = 0x00B7
	wmSetFont     = 0x0030
	dtLeft        = 0x00000000
	dtTop         = 0x00000000
	dtSingleLine  = 0x00000020
	dtVCenter     = 0x00000004
	dtWordBreak   = 0x00000010
	transparent   = 1

	colorWindow = 5

	idChoose          = 1001
	idStart           = 1002
	idFFmpegDir       = 1003
	idWhisperDir      = 1004
	idDownloadFFmpeg  = 1005
	idDownloadWhisper = 1006

	defaultFFmpegDir  = `D:\Tools\ffmpeg`
	defaultWhisperDir = `D:\Models\asr\whisper.cpp`
	ffmpegZipURL      = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"
	whisperZipURL     = "https://github.com/ggml-org/whisper.cpp/releases/download/v1.8.6/whisper-bin-x64.zip"
	whisperModelURL   = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin"

	supportedMediaLabel   = "mp4, mkv, flv, mov, avi, webm, mp3, ogg, aac"
	supportedMediaPattern = "*.mp4;*.mkv;*.flv;*.mov;*.avi;*.webm;*.mp3;*.ogg;*.aac"
)

type point struct {
	x int32
	y int32
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type rect struct {
	left   int32
	top    int32
	right  int32
	bottom int32
}

type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type drawItemStruct struct {
	ctlType    uint32
	ctlID      uint32
	itemID     uint32
	itemAction uint32
	itemState  uint32
	hwndItem   uintptr
	hdc        uintptr
	rcItem     rect
	itemData   uintptr
}

type openFileName struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

type browseInfo struct {
	hwndOwner      uintptr
	pidlRoot       uintptr
	pszDisplayName *uint16
	lpszTitle      *uint16
	ulFlags        uint32
	lpfn           uintptr
	lParam         uintptr
	iImage         int32
}

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")

	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procUpdateWindow        = user32.NewProc("UpdateWindow")
	procSetWindowText       = user32.NewProc("SetWindowTextW")
	procGetWindowText       = user32.NewProc("GetWindowTextW")
	procSendMessage         = user32.NewProc("SendMessageW")
	procPostMessage         = user32.NewProc("PostMessageW")
	procEnableWindow        = user32.NewProc("EnableWindow")
	procGetOpenFileName     = comdlg32.NewProc("GetOpenFileNameW")
	procCommDlgExtError     = comdlg32.NewProc("CommDlgExtendedError")
	procSHBrowseForFolder   = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDList = shell32.NewProc("SHGetPathFromIDListW")
	procCoTaskMemFree       = ole32.NewProc("CoTaskMemFree")
	procCoInitializeEx      = ole32.NewProc("CoInitializeEx")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	procLoadCursor          = user32.NewProc("LoadCursorW")
	procLoadIcon            = user32.NewProc("LoadIconW")
	procBeginPaint          = user32.NewProc("BeginPaint")
	procEndPaint            = user32.NewProc("EndPaint")
	procCreateFont          = gdi32.NewProc("CreateFontW")
	procCreatePen           = gdi32.NewProc("CreatePen")
	procCreateSolidBrush    = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject        = gdi32.NewProc("DeleteObject")
	procSelectObject        = gdi32.NewProc("SelectObject")
	procSetBkMode           = gdi32.NewProc("SetBkMode")
	procSetTextColor        = gdi32.NewProc("SetTextColor")
	procRoundRect           = gdi32.NewProc("RoundRect")
	procFillRect            = user32.NewProc("FillRect")
	procDrawText            = user32.NewProc("DrawTextW")
	procInitCommonCtrls     = comctl32.NewProc("InitCommonControls")

	wndProcCallback uintptr

	mainWindow         uintptr
	fileText           uintptr
	outputText         uintptr
	statusText         uintptr
	ffmpegDirText      uintptr
	whisperDirText     uintptr
	progress           uintptr
	startBtn           uintptr
	chooseBtn          uintptr
	ffmpegDirBtn       uintptr
	whisperDirBtn      uintptr
	downloadFFmpegBtn  uintptr
	downloadWhisperBtn uintptr

	selectedVideo string
	ffmpegDir     string
	whisperDir    string
	isRunning     bool
	uiFont        uintptr
	titleFont     uintptr
	sectionFont   uintptr
	smallFont     uintptr
	bgBrush       uintptr
	panelBrush    uintptr
	fieldBrush    uintptr

	payloadMu sync.Mutex
	payloadID uintptr
	payloads  = map[uintptr]string{}
)

func main() {
	runtime.LockOSThread()
	procCoInitializeEx.Call(0, 2)
	procInitCommonCtrls.Call()

	hInstance, _, _ := procGetModuleHandle.Call(0)
	className := utf16Ptr("Video2TextNativeWindow")
	wndProcCallback = syscall.NewCallback(windowProc)

	cursor, _, _ := procLoadCursor.Call(0, 32512)
	icon, _, _ := procLoadIcon.Call(hInstance, 1)
	wc := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		lpfnWndProc:   wndProcCallback,
		hInstance:     hInstance,
		hIcon:         icon,
		hCursor:       cursor,
		hbrBackground: colorWindow + 1,
		lpszClassName: className,
		hIconSm:       icon,
	}
	if ret, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); ret == 0 {
		exitError("register window failed", err)
	}

	mainWindow = createWindow(0, className, appTitle, wsOverlappedWindow|wsVisible|wsClipChildren, cwUseDefault, cwUseDefault, 940, 640, 0, 0, hInstance, 0)
	if mainWindow == 0 {
		exitError("create window failed", syscall.GetLastError())
	}

	procShowWindow.Call(mainWindow, swShow)
	procUpdateWindow.Call(mainWindow)

	var m msg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func windowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	defer func() {
		if r := recover(); r != nil {
			setStatusUI(fmt.Sprintf("内部错误：%v", r))
		}
	}()

	switch message {
	case wmCreate:
		createControls(hwnd)
		return 0
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		paintUI(hwnd)
		return 0
	case wmDrawItem:
		drawButton((*drawItemStruct)(unsafe.Pointer(lParam)))
		return 1
	case wmCtlColorStatic:
		procSetBkMode.Call(wParam, transparent)
		procSetTextColor.Call(wParam, rgb(28, 32, 38))
		return fieldBrush
	case wmCtlColorEdit:
		procSetBkMode.Call(wParam, transparent)
		procSetTextColor.Call(wParam, rgb(36, 41, 49))
		return fieldBrush
	case wmCommand:
		id := int(wParam & 0xffff)
		switch id {
		case idChoose:
			if isRunning {
				return 0
			}
			path, ok, err := chooseMedia(hwnd)
			if err != nil {
				setStatusUI("文件选择失败：" + err.Error())
				return 0
			}
			if ok {
				selectedVideo = path
				setText(fileText, path)
				setText(outputText, outputPath(path))
				setStatusUI("已选择媒体文件。")
				setProgressUI(0)
			}
			return 0
		case idStart:
			if selectedVideo == "" || isRunning {
				return 0
			}
			isRunning = true
			setBusy(true)
			setStatusUI("开始转写。")
			go runTranscription(selectedVideo)
			return 0
		case idFFmpegDir:
			if path, ok := chooseFolder(hwnd, "选择 ffmpeg 安装目录"); ok {
				ffmpegDir = path
				setText(ffmpegDirText, path)
				setStatusUI("已设置 ffmpeg 目录。")
			}
			return 0
		case idWhisperDir:
			if path, ok := chooseFolder(hwnd, "选择 whisper.cpp 目录"); ok {
				whisperDir = path
				setText(whisperDirText, path)
				setStatusUI("已设置 whisper.cpp 目录。")
			}
			return 0
		case idDownloadFFmpeg:
			if isRunning {
				return 0
			}
			isRunning = true
			setBusy(true)
			go downloadFFmpeg()
			return 0
		case idDownloadWhisper:
			if isRunning {
				return 0
			}
			isRunning = true
			setBusy(true)
			go downloadWhisperAssets()
			return 0
		}
	case msgLog:
		setStatusUI(takePayload(wParam))
		return 0
	case msgProgress:
		setProgressUI(int(wParam))
		return 0
	case msgDone:
		isRunning = false
		setBusy(false)
		return 0
	case wmClose:
		if isRunning {
			setStatusUI("任务仍在运行，请等待完成。")
			return 0
		}
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}

func createControls(hwnd uintptr) {
	hInstance, _, _ := procGetModuleHandle.Call(0)
	initTheme()
	ffmpegDir = initialFFmpegDir()
	whisperDir = initialWhisperDir()

	fileText = createEdit(hwnd, "未选择媒体文件", 72, 186, 488, 30)
	chooseBtn = createButton(hwnd, "选择文件", 72, 258, 124, 40, idChoose, hInstance)
	startBtn = createButton(hwnd, "开始转写", 210, 258, 124, 40, idStart, hInstance)

	outputText = createEdit(hwnd, "选择文件后自动生成同目录 txt", 72, 350, 488, 30)

	ffmpegDirText = createEdit(hwnd, ffmpegDir, 632, 186, 218, 30)
	ffmpegDirBtn = createButton(hwnd, "目录", 632, 226, 74, 36, idFFmpegDir, hInstance)
	downloadFFmpegBtn = createButton(hwnd, "下载", 716, 226, 74, 36, idDownloadFFmpeg, hInstance)

	whisperDirText = createEdit(hwnd, whisperDir, 632, 326, 218, 30)
	whisperDirBtn = createButton(hwnd, "目录", 632, 366, 74, 36, idWhisperDir, hInstance)
	downloadWhisperBtn = createButton(hwnd, "下载", 716, 366, 74, 36, idDownloadWhisper, hInstance)

	progress = createWindow(0, utf16Ptr("msctls_progress32"), "", wsChild|wsVisible, 56, 520, 826, 14, hwnd, 0, hInstance, 0)
	setProgressUI(0)

	statusText = createStatic(hwnd, "就绪。支持 "+supportedMediaLabel+"，自动按实际语种输出。", 56, 548, 826, 24)
}

func createStatic(parent uintptr, text string, x, y, w, height int32) uintptr {
	hInstance, _, _ := procGetModuleHandle.Call(0)
	ctrl := createWindow(0, utf16Ptr("STATIC"), text, wsChild|wsVisible, x, y, w, height, parent, 0, hInstance, 0)
	applyFont(ctrl)
	return ctrl
}

func createButton(parent uintptr, text string, x, y, w, height int32, id int, hInstance uintptr) uintptr {
	ctrl := createWindow(0, utf16Ptr("BUTTON"), text, wsChild|wsVisible|wsTabStop|bsPushButton|bsOwnerDraw, x, y, w, height, parent, uintptr(id), hInstance, 0)
	applyFont(ctrl)
	return ctrl
}

func createEdit(parent uintptr, text string, x, y, w, height int32) uintptr {
	hInstance, _, _ := procGetModuleHandle.Call(0)
	ctrl := createWindow(0, utf16Ptr("EDIT"), text, wsChild|wsVisible|esReadonly|esAutoHScroll, x, y, w, height, parent, 0, hInstance, 0)
	applyFont(ctrl)
	return ctrl
}

func initTheme() {
	uiFont = createFont("Microsoft YaHei UI", -16, 400)
	titleFont = createFont("Microsoft YaHei UI", -34, 650)
	sectionFont = createFont("Microsoft YaHei UI", -18, 650)
	smallFont = createFont("Microsoft YaHei UI", -13, 400)
	bgBrush = createBrush(244, 245, 247)
	panelBrush = createBrush(255, 255, 255)
	fieldBrush = createBrush(250, 251, 253)
}

func paintUI(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

	fill(hdc, 0, 0, 940, 640, bgBrush)
	drawText(hdc, "Video2Text", 40, 30, 300, 42, titleFont, rgb(20, 23, 29), dtLeft|dtTop|dtSingleLine)
	drawText(hdc, "本地音视频转文字稿", 42, 76, 220, 24, smallFont, rgb(93, 99, 111), dtLeft|dtTop|dtSingleLine)
	drawPill(hdc, 708, 38, 176, 34, "本地 whisper.cpp")

	drawCard(hdc, 40, 124, 552, 306)
	drawCard(hdc, 612, 124, 272, 306)
	drawCard(hdc, 40, 464, 844, 92)

	drawText(hdc, "选择音视频", 72, 152, 220, 28, sectionFont, rgb(28, 32, 38), dtLeft|dtTop|dtSingleLine)
	drawText(hdc, "选择 "+supportedMediaLabel+" 等文件，转写结果会保存为同目录同名 txt。", 72, 180, 470, 24, smallFont, rgb(105, 112, 124), dtLeft|dtTop|dtSingleLine)
	drawField(hdc, 64, 178, 512, 48)

	drawText(hdc, "输出位置", 72, 308, 220, 28, sectionFont, rgb(28, 32, 38), dtLeft|dtTop|dtSingleLine)
	drawText(hdc, "无需额外设置，按视频文件名自动生成。", 72, 336, 420, 24, smallFont, rgb(105, 112, 124), dtLeft|dtTop|dtSingleLine)
	drawField(hdc, 64, 342, 512, 48)

	drawText(hdc, "本地能力", 632, 152, 160, 28, sectionFont, rgb(28, 32, 38), dtLeft|dtTop|dtSingleLine)
	drawText(hdc, "ffmpeg 目录", 632, 174, 160, 22, smallFont, rgb(105, 112, 124), dtLeft|dtTop|dtSingleLine)
	drawField(hdc, 624, 178, 242, 48)
	drawText(hdc, "whisper.cpp 目录", 632, 314, 160, 22, smallFont, rgb(105, 112, 124), dtLeft|dtTop|dtSingleLine)
	drawField(hdc, 624, 318, 242, 48)
	drawText(hdc, "下载后会写入用户级环境变量，新的终端会自动生效。", 632, 414, 214, 34, smallFont, rgb(105, 112, 124), dtLeft|dtTop|dtWordBreak)

	drawText(hdc, "当前状态", 56, 480, 160, 26, sectionFont, rgb(28, 32, 38), dtLeft|dtTop|dtSingleLine)
}

func drawCard(hdc uintptr, x, y, w, h int32) {
	pen := createPen(0, 1, rgb(225, 229, 235))
	oldPen, _, _ := procSelectObject.Call(hdc, pen)
	oldBrush, _, _ := procSelectObject.Call(hdc, panelBrush)
	procRoundRect.Call(hdc, uintptr(x), uintptr(y), uintptr(x+w), uintptr(y+h), 22, 22)
	procSelectObject.Call(hdc, oldBrush)
	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(pen)
}

func drawField(hdc uintptr, x, y, w, h int32) {
	pen := createPen(0, 1, rgb(232, 236, 242))
	oldPen, _, _ := procSelectObject.Call(hdc, pen)
	oldBrush, _, _ := procSelectObject.Call(hdc, fieldBrush)
	procRoundRect.Call(hdc, uintptr(x), uintptr(y), uintptr(x+w), uintptr(y+h), 14, 14)
	procSelectObject.Call(hdc, oldBrush)
	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(pen)
}

func drawPill(hdc uintptr, x, y, w, h int32, text string) {
	brush := createBrush(235, 247, 241)
	pen := createPen(0, 1, rgb(206, 232, 220))
	oldPen, _, _ := procSelectObject.Call(hdc, pen)
	oldBrush, _, _ := procSelectObject.Call(hdc, brush)
	procRoundRect.Call(hdc, uintptr(x), uintptr(y), uintptr(x+w), uintptr(y+h), 18, 18)
	procSelectObject.Call(hdc, oldBrush)
	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(brush)
	procDeleteObject.Call(pen)
	drawText(hdc, text, x+18, y+8, w-30, h-8, smallFont, rgb(34, 132, 92), dtLeft|dtTop|dtSingleLine)
}

func drawStatusLine(hdc uintptr, x, y int32, index, text string) {
	brush := createBrush(239, 243, 248)
	pen := createPen(0, 1, rgb(224, 229, 236))
	oldPen, _, _ := procSelectObject.Call(hdc, pen)
	oldBrush, _, _ := procSelectObject.Call(hdc, brush)
	procRoundRect.Call(hdc, uintptr(x), uintptr(y), uintptr(x+30), uintptr(y+30), 15, 15)
	procSelectObject.Call(hdc, oldBrush)
	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(brush)
	procDeleteObject.Call(pen)
	drawText(hdc, index, x+10, y+6, 20, 18, smallFont, rgb(79, 89, 105), dtLeft|dtTop|dtSingleLine)
	drawText(hdc, text, x+44, y+6, 150, 22, uiFont, rgb(42, 48, 58), dtLeft|dtTop|dtSingleLine)
}

func drawButton(item *drawItemStruct) {
	if item == nil || item.hdc == 0 {
		return
	}
	disabled := item.itemState&0x0004 != 0
	primary := item.ctlID == idStart

	var bg, border, fg uintptr
	switch {
	case disabled:
		bg = rgb(232, 235, 240)
		border = rgb(220, 224, 231)
		fg = rgb(132, 140, 152)
	case primary:
		bg = rgb(0, 113, 227)
		border = rgb(0, 101, 204)
		fg = rgb(255, 255, 255)
	default:
		bg = rgb(249, 250, 252)
		border = rgb(218, 223, 231)
		fg = rgb(32, 37, 45)
	}

	brush, _, _ := procCreateSolidBrush.Call(bg)
	pen := createPen(0, 1, border)
	oldPen, _, _ := procSelectObject.Call(item.hdc, pen)
	oldBrush, _, _ := procSelectObject.Call(item.hdc, brush)
	procRoundRect.Call(
		item.hdc,
		uintptr(item.rcItem.left),
		uintptr(item.rcItem.top),
		uintptr(item.rcItem.right),
		uintptr(item.rcItem.bottom),
		14,
		14,
	)
	procSelectObject.Call(item.hdc, oldBrush)
	procSelectObject.Call(item.hdc, oldPen)
	procDeleteObject.Call(brush)
	procDeleteObject.Call(pen)

	text := getWindowTextUI(item.hwndItem)
	r := item.rcItem
	drawText(item.hdc, text, r.left, r.top+1, r.right-r.left, r.bottom-r.top, uiFont, fg, dtLeft|dtVCenter|dtSingleLine|0x00000001)
}

func fill(hdc uintptr, x, y, w, h int32, brush uintptr) {
	r := rect{x, y, x + w, y + h}
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), brush)
}

func drawText(hdc uintptr, text string, x, y, w, h int32, font uintptr, color uintptr, flags uintptr) {
	oldFont, _, _ := procSelectObject.Call(hdc, font)
	procSetBkMode.Call(hdc, transparent)
	procSetTextColor.Call(hdc, color)
	r := rect{x, y, x + w, y + h}
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(utf16Ptr(text))), ^uintptr(0), uintptr(unsafe.Pointer(&r)), flags)
	procSelectObject.Call(hdc, oldFont)
}

func rgb(r, g, b byte) uintptr {
	return uintptr(uint32(r) | uint32(g)<<8 | uint32(b)<<16)
}

func createBrush(r, g, b byte) uintptr {
	brush, _, _ := procCreateSolidBrush.Call(rgb(r, g, b))
	return brush
}

func createPen(style, width int32, color uintptr) uintptr {
	pen, _, _ := procCreatePen.Call(uintptr(style), uintptr(width), color)
	return pen
}

func createWindow(exStyle uintptr, className *uint16, title string, style uint32, x, y, w, h int32, parent, menu, instance, param uintptr) uintptr {
	ret, _, _ := procCreateWindowEx.Call(
		exStyle,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(utf16Ptr(title))),
		uintptr(style),
		uintptr(uint32(x)),
		uintptr(uint32(y)),
		uintptr(uint32(w)),
		uintptr(uint32(h)),
		parent,
		menu,
		instance,
		param,
	)
	return ret
}

func chooseMedia(hwnd uintptr) (string, bool, error) {
	buf := make([]uint16, 4096)
	filter := makeDialogFilter(
		"Media files ("+supportedMediaPattern+")",
		supportedMediaPattern,
		"Video files (*.mp4;*.mkv;*.flv;*.mov;*.avi;*.webm)",
		"*.mp4;*.mkv;*.flv;*.mov;*.avi;*.webm",
		"Audio files (*.mp3;*.ogg;*.aac)",
		"*.mp3;*.ogg;*.aac",
		"All files (*.*)",
		"*.*",
	)
	title := syscall.StringToUTF16("Choose media file")
	ofn := openFileName{
		lStructSize: uint32(unsafe.Sizeof(openFileName{})),
		hwndOwner:   hwnd,
		lpstrFilter: &filter[0],
		lpstrFile:   &buf[0],
		nMaxFile:    uint32(len(buf)),
		lpstrTitle:  &title[0],
		flags:       ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnNoChangeDir,
	}
	ret, _, _ := procGetOpenFileName.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret == 0 {
		code, _, _ := procCommDlgExtError.Call()
		if code != 0 {
			return "", false, fmt.Errorf("CommDlgExtendedError=0x%X", code)
		}
		return "", false, nil
	}
	return syscall.UTF16ToString(buf), true, nil
}

func makeDialogFilter(parts ...string) []uint16 {
	var out []uint16
	for _, part := range parts {
		out = append(out, utf16.Encode([]rune(part))...)
		out = append(out, 0)
	}
	return append(out, 0)
}

func chooseFolder(hwnd uintptr, title string) (string, bool) {
	displayName := make([]uint16, 260)
	titleUTF := syscall.StringToUTF16(title)
	bi := browseInfo{
		hwndOwner:      hwnd,
		pszDisplayName: &displayName[0],
		lpszTitle:      &titleUTF[0],
		ulFlags:        0x0001 | 0x0040,
	}
	pidl, _, _ := procSHBrowseForFolder.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", false
	}
	defer procCoTaskMemFree.Call(pidl)
	pathBuf := make([]uint16, 32768)
	ret, _, _ := procSHGetPathFromIDList.Call(pidl, uintptr(unsafe.Pointer(&pathBuf[0])))
	if ret == 0 {
		return "", false
	}
	return syscall.UTF16ToString(pathBuf), true
}

func runTranscription(video string) {
	defer postDone()

	out := outputPath(video)
	tempDir, err := os.MkdirTemp("", "video2text-*")
	if err != nil {
		fail("Create temp directory failed", err)
		return
	}
	defer os.RemoveAll(tempDir)

	audio := filepath.Join(tempDir, "audio.wav")
	postProgress(10)
	postLog("Extracting audio with ffmpeg.")
	ffmpeg, err := resolveFFmpeg()
	if err != nil {
		fail("ffmpeg not found", err)
		return
	}
	if err := runCommand(ffmpeg, "-y", "-i", video, "-vn", "-acodec", "pcm_s16le", "-ar", "16000", "-ac", "1", audio); err != nil {
		fail("ffmpeg failed", err)
		return
	}

	postProgress(35)
	postLog("Running whisper.cpp.")
	transcript, err := runWhisperCPP(audio, tempDir)
	if err != nil {
		fail("whisper.cpp failed", err)
		return
	}

	postProgress(90)
	if strings.TrimSpace(transcript) == "" {
		fail("Empty transcript", errors.New("whisper.cpp produced no text"))
		return
	}
	if err := os.WriteFile(out, []byte(transcript), 0644); err != nil {
		fail("Write transcript failed", err)
		return
	}

	postProgress(100)
	postLog("Done: " + out)
}

func downloadFFmpeg() {
	defer postDone()
	postProgress(5)
	postLog("正在下载 ffmpeg...")
	if err := os.MkdirAll(ffmpegDir, 0755); err != nil {
		fail("创建 ffmpeg 目录失败", err)
		return
	}

	zipPath := filepath.Join(ffmpegDir, "ffmpeg-release-essentials.zip")
	if err := downloadFile(ffmpegZipURL, zipPath); err != nil {
		fail("下载 ffmpeg 失败", err)
		return
	}
	postProgress(55)
	postLog("正在解压 ffmpeg...")
	if err := unzip(zipPath, ffmpegDir); err != nil {
		fail("解压 ffmpeg 失败", err)
		return
	}

	ffmpegExe, err := findFile(ffmpegDir, "ffmpeg.exe")
	if err != nil {
		fail("查找 ffmpeg.exe 失败", err)
		return
	}
	if err := setUserEnv("FFMPEG_CMD", ffmpegExe); err != nil {
		fail("写入 FFMPEG_CMD 失败", err)
		return
	}
	postProgress(100)
	postLog("ffmpeg 已安装，并写入 FFMPEG_CMD。")
}

func downloadWhisperAssets() {
	defer postDone()
	postProgress(5)
	postLog("正在下载 whisper.cpp...")
	if err := os.MkdirAll(whisperDir, 0755); err != nil {
		fail("创建 whisper.cpp 目录失败", err)
		return
	}

	zipPath := filepath.Join(whisperDir, "whisper-bin-x64-v1.8.6.zip")
	if err := downloadFile(whisperZipURL, zipPath); err != nil {
		fail("下载 whisper.cpp 失败", err)
		return
	}
	postProgress(35)
	postLog("正在解压 whisper.cpp...")
	if err := unzip(zipPath, whisperDir); err != nil {
		fail("解压 whisper.cpp 失败", err)
		return
	}

	modelPath := filepath.Join(whisperDir, "ggml-small.bin")
	postProgress(55)
	postLog("正在下载 Whisper 中文可用模型...")
	if !fileExists(modelPath) {
		if err := downloadFile(whisperModelURL, modelPath); err != nil {
			fail("下载 Whisper 模型失败", err)
			return
		}
	}

	whisperExe, err := findAnyFile(whisperDir, []string{"whisper-cli.exe", "main.exe"})
	if err != nil {
		fail("查找 whisper.cpp 可执行文件失败", err)
		return
	}
	if err := setUserEnv("WHISPER_CPP_EXE", whisperExe); err != nil {
		fail("写入 WHISPER_CPP_EXE 失败", err)
		return
	}
	if err := setUserEnv("WHISPER_MODEL", modelPath); err != nil {
		fail("写入 WHISPER_MODEL 失败", err)
		return
	}
	postProgress(100)
	postLog("whisper.cpp 和模型已安装，并写入环境变量。")
}

func runWhisperCPP(audio, tempDir string) (string, error) {
	bin, err := resolveWhisperBinary()
	if err != nil {
		return "", err
	}
	model, err := resolveWhisperModel()
	if err != nil {
		return "", err
	}

	prefix := filepath.Join(tempDir, "transcript")
	args := []string{
		"-m", model,
		"-f", audio,
		"-l", "auto",
		"-otxt",
		"-of", prefix,
	}
	if err := runCommand(bin, args...); err != nil {
		return "", err
	}

	txt := prefix + ".txt"
	data, err := os.ReadFile(txt)
	if err != nil {
		return "", fmt.Errorf("read whisper output %s: %w", txt, err)
	}
	return string(data), nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				postLog(line)
			}
		}
	}
	if err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func downloadFile(url, target string) error {
	if fileExists(target) {
		return nil
	}
	tmp := target + ".download"
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	file, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, target)
}

func unzip(zipPath, dest string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	for _, file := range reader.File {
		target := filepath.Join(dest, file.Name)
		targetAbs, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		if targetAbs != destAbs && !strings.HasPrefix(targetAbs, destAbs+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe zip path: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.Create(target)
		if err != nil {
			_ = src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		closeSrcErr := src.Close()
		closeDstErr := dst.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeSrcErr != nil {
			return closeSrcErr
		}
		if closeDstErr != nil {
			return closeDstErr
		}
	}
	return nil
}

func findFile(root, name string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if found == "" && !entry.IsDir() && strings.EqualFold(entry.Name(), name) {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("%s not found in %s", name, root)
	}
	return found, nil
}

func findAnyFile(root string, names []string) (string, error) {
	for _, name := range names {
		if found, err := findFile(root, name); err == nil {
			return found, nil
		}
	}
	return "", fmt.Errorf("%s not found in %s", strings.Join(names, " or "), root)
}

func setUserEnv(name, value string) error {
	_ = os.Setenv(name, value)
	cmd := exec.Command("setx", name, value)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func resolveFFmpeg() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("FFMPEG_CMD")); configured != "" {
		return configured, nil
	}
	if ffmpegDir != "" {
		if found, err := findFile(ffmpegDir, "ffmpeg.exe"); err == nil {
			return found, nil
		}
	}
	if exePath, err := os.Executable(); err == nil {
		localPath := filepath.Join(filepath.Dir(exePath), "ffmpeg.exe")
		if fileExists(localPath) {
			return localPath, nil
		}
	}
	return exec.LookPath("ffmpeg")
}

func resolveWhisperBinary() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("WHISPER_CPP_EXE")); configured != "" {
		if fileExists(configured) {
			return configured, nil
		}
		return "", fmt.Errorf("WHISPER_CPP_EXE does not exist: %s", configured)
	}

	candidates := []string{
		filepath.Join(whisperDir, "whisper-cli.exe"),
		filepath.Join(whisperDir, "main.exe"),
		filepath.Join(whisperDir, "Release", "whisper-cli.exe"),
		filepath.Join(whisperDir, "Release", "main.exe"),
		filepath.Join(whisperDir, "bin", "whisper-cli.exe"),
		filepath.Join(whisperDir, "bin", "main.exe"),
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate, nil
		}
	}

	if found, err := exec.LookPath("whisper-cli"); err == nil {
		return found, nil
	}
	if found, err := exec.LookPath("main"); err == nil {
		return found, nil
	}

	return "", fmt.Errorf("missing whisper.cpp executable in %s", whisperDir)
}

func resolveWhisperModel() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("WHISPER_MODEL")); configured != "" {
		if fileExists(configured) {
			return configured, nil
		}
		return "", fmt.Errorf("WHISPER_MODEL does not exist: %s", configured)
	}

	candidates := []string{
		filepath.Join(whisperDir, "ggml-medium.bin"),
		filepath.Join(whisperDir, "ggml-small.bin"),
		filepath.Join(whisperDir, "ggml-large-v3.bin"),
		filepath.Join(whisperDir, "ggml-large-v2.bin"),
		filepath.Join(whisperDir, "ggml-medium.en.bin"),
		filepath.Join(whisperDir, "ggml-small.en.bin"),
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			if strings.Contains(filepath.Base(candidate), ".en.") {
				postLog("Warning: using English-only Whisper model. Chinese transcription may be poor.")
			}
			return candidate, nil
		}
	}
	return "", fmt.Errorf("missing Whisper model in %s", whisperDir)
}

func initialFFmpegDir() string {
	if configured := strings.TrimSpace(os.Getenv("FFMPEG_CMD")); configured != "" {
		return filepath.Dir(configured)
	}
	if fileExists(filepath.Join(defaultFFmpegDir, "ffmpeg.exe")) {
		return defaultFFmpegDir
	}
	if found, err := exec.LookPath("ffmpeg"); err == nil {
		return filepath.Dir(found)
	}
	return defaultFFmpegDir
}

func initialWhisperDir() string {
	if configured := strings.TrimSpace(os.Getenv("WHISPER_CPP_EXE")); configured != "" {
		dir := filepath.Dir(configured)
		if strings.EqualFold(filepath.Base(dir), "Release") || strings.EqualFold(filepath.Base(dir), "bin") {
			return filepath.Dir(dir)
		}
		return dir
	}
	if fileExists(filepath.Join(defaultWhisperDir, "Release", "whisper-cli.exe")) || fileExists(filepath.Join(defaultWhisperDir, "whisper-cli.exe")) {
		return defaultWhisperDir
	}
	return defaultWhisperDir
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func outputPath(video string) string {
	ext := filepath.Ext(video)
	return strings.TrimSuffix(video, ext) + ".txt"
}

func postLog(text string) {
	if mainWindow == 0 {
		return
	}
	id := storePayload(text)
	procPostMessage.Call(mainWindow, msgLog, id, 0)
}

func postProgress(value int) {
	if mainWindow != 0 {
		procPostMessage.Call(mainWindow, msgProgress, uintptr(value), 0)
	}
}

func postDone() {
	if mainWindow != 0 {
		procPostMessage.Call(mainWindow, msgDone, 0, 0)
	}
}

func storePayload(text string) uintptr {
	payloadMu.Lock()
	defer payloadMu.Unlock()
	payloadID++
	payloads[payloadID] = text
	return payloadID
}

func takePayload(id uintptr) string {
	payloadMu.Lock()
	defer payloadMu.Unlock()
	text := payloads[id]
	delete(payloads, id)
	return text
}

func setStatusUI(text string) {
	if statusText == 0 {
		return
	}
	setText(statusText, time.Now().Format("15:04:05")+"  "+text)
}

func setBusy(busy bool) {
	enable(startBtn, !busy)
	enable(chooseBtn, !busy)
	enable(ffmpegDirBtn, !busy)
	enable(whisperDirBtn, !busy)
	enable(downloadFFmpegBtn, !busy)
	enable(downloadWhisperBtn, !busy)
}

func fail(prefix string, err error) {
	postProgress(0)
	postLog(prefix + ": " + err.Error())
}

func setText(hwnd uintptr, text string) {
	procSetWindowText.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr(text))))
}

func getWindowTextUI(hwnd uintptr) string {
	buf := make([]uint16, 32768)
	procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}

func setProgressUI(value int) {
	if progress == 0 {
		return
	}
	procSendMessage.Call(progress, pbmSetRange, 0, uintptr(100<<16))
	procSendMessage.Call(progress, pbmSetPos, uintptr(value), 0)
}

func enable(hwnd uintptr, yes bool) {
	v := uintptr(0)
	if yes {
		v = 1
	}
	procEnableWindow.Call(hwnd, v)
}

func applyFont(hwnd uintptr) {
	if hwnd != 0 && uiFont != 0 {
		procSendMessage.Call(hwnd, wmSetFont, uiFont, 1)
	}
}

func createUIFont() uintptr {
	return createFont("Microsoft YaHei UI", -16, 400)
}

func createFont(face string, height int32, weight int32) uintptr {
	name := utf16Ptr(face)
	font, _, _ := procCreateFont.Call(
		uintptr(uint32(height)),
		0, 0, 0,
		uintptr(uint32(weight)),
		0, 0, 0,
		1,
		0, 0, 5, 0,
		uintptr(unsafe.Pointer(name)),
	)
	return font
}

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func exitError(prefix string, err error) {
	_, _ = fmt.Fprintf(os.Stderr, "%s: %v\n", prefix, err)
	os.Exit(1)
}
