// Package goide is a small end-to-end Go IDE example. Process and protocol
// clients live here; nativeForms remains a UI toolkit.
package goide

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/editor"
	"github.com/banditmoscow1337/nativeForms/platform"
)

type DesktopHost interface {
	platform.FileDialogs
	Capabilities() platform.Capabilities
}

type openDocument struct {
	path string
	document *editor.Document
	view *ui.CodeEditor
	lastText string
	lspVersion int
	dirty bool
	shortcuts []ui.ShortcutID
}

type App struct {
	manager *ui.Manager
	host DesktopHost
	Root ui.Component
	project *projectTree
	fileTree *ui.VirtualTree
	editorTabs *ui.Tabs
	output *ui.OutputPanel
	status *ui.Label
	diagnostics *diagnosticsModel
	diagnosticsView *ui.VirtualTable
	diagnosticByURI map[string][]lspDiagnostic
	stack *stackModel
	stackView *ui.VirtualTable
	variables *variablesModel
	variablesView *ui.VirtualTable
	files map[string]*openDocument
	breakpoints map[string]map[int]bool
	lsp *lspClient
	debug *debugSession
	closed bool
}

func New(manager *ui.Manager, host DesktopHost) *App {
	a := &App{manager:manager,host:host,project:newProjectTree(""),
		files:make(map[string]*openDocument),breakpoints:make(map[string]map[int]bool),
		diagnosticByURI:make(map[string][]lspDiagnostic),
		diagnostics:&diagnosticsModel{},stack:&stackModel{},variables:&variablesModel{},
	}
	a.status = ui.NewLabel("Откройте Go-модуль")
	a.output = ui.NewOutputPanel()
	a.fileTree = ui.NewVirtualTree(a.project)
	a.fileTree.OnSelect = func(path string) {
		info,err := os.Stat(path)
		if err == nil && !info.IsDir() { if err := a.openFile(path); err != nil { a.status.SetText(err.Error()) } }
	}
	a.editorTabs = ui.NewTabs(nil,"",func(string){})
	a.editorTabs.OnCloseRequest = func(path string) bool {
		file := a.files[path]
		if file == nil { return true }
		if file.dirty { a.status.SetText("Сначала сохраните "+fileName(path)); return false }
		if a.lsp != nil { a.lsp.closeFile(path) }
		for _,id := range file.shortcuts { manager.RemoveShortcut(id) }
		delete(a.files,path)
		return true
	}
	a.diagnosticsView = ui.NewVirtualTable(a.diagnostics)
	a.diagnosticsView.OnSelect = func(id string) {
		for _,row := range a.diagnostics.rows {
			if row.id != id { continue }
			if err := a.openFile(row.path); err != nil { a.status.SetText(err.Error()); return }
			file := a.files[row.path]
			at,err := file.document.ByteOffset(editor.Position{Line:row.line,Character:row.column})
			if err == nil { file.view.ScrollToLine(row.line); file.view.SetSelection(at,at) }
			return
		}
	}
	a.stackView = ui.NewVirtualTable(a.stack)
	a.stackView.OnSelect = func(id string) {
		for _,row := range a.stack.rows {
			if strconv.Itoa(row.id)==id {
				a.showFrame(row)
				if a.debug!=nil { a.debug.fetchScopes(row.id) }
				return
			}
		}
	}
	a.variablesView = ui.NewVirtualTable(a.variables)
	bottom := ui.NewTabs([]ui.Tab{
		{ID:"problems",Title:"Problems",Content:a.diagnosticsView,NonClosable:true},
		{ID:"output",Title:"Output",Content:a.output,NonClosable:true},
	},"problems",nil)
	debugViews := ui.NewTabs([]ui.Tab{
		{ID:"stack",Title:"Call stack",Content:a.stackView,NonClosable:true},
		{ID:"variables",Title:"Variables",Content:a.variablesView,NonClosable:true},
	},"stack",nil)
	center := ui.NewSplitPane(ui.Vertical,a.editorTabs,bottom)
	center.Ratio=0.72
	work := ui.NewSplitPane(ui.Horizontal,center,debugViews)
	work.Ratio=0.76
	layout := ui.NewSplitPane(ui.Horizontal,a.fileTree,work)
	layout.Ratio=0.19
	layout.SetFlex(1)
	bar := ui.NewStack(ui.Horizontal,6)
	bar.Padding=ui.Symmetric(8,5)
	bar.Add(ui.NewButton("Open folder",func(){
		path,err := host.OpenFolder()
		if err != nil { a.status.SetText(err.Error()) } else if path!="" {
			if err := a.OpenProject(path); err != nil { a.status.SetText(err.Error()) }
		}
	}))
	bar.Add(ui.NewButton("Save",func(){ if err := a.SaveActive(); err != nil { a.status.SetText(err.Error()) } }))
	bar.Add(ui.NewButton("Complete",a.Complete))
	bar.Add(ui.NewButton("Debug",func(){ a.StartDebug("debug") }))
	bar.Add(ui.NewButton("Test",func(){ a.StartDebug("test") }))
	bar.Add(ui.NewButton("Continue",func(){ a.debugControl("continue") }))
	bar.Add(ui.NewButton("Next",func(){ a.debugControl("next") }))
	bar.Add(ui.NewButton("Step in",func(){ a.debugControl("stepIn") }))
	bar.Add(ui.NewButton("Step out",func(){ a.debugControl("stepOut") }))
	bar.Add(ui.NewButton("Stop",a.StopDebug))
	if !host.Capabilities().FileDialog { bar.Children()[0].UIState().SetEnabled(false) }
	root := ui.NewStack(ui.Vertical,0)
	root.Add(bar,layout,a.status)
	a.Root=root
	manager.AddShortcut(ui.Shortcut{Key:ui.KeyS,Mods:ui.ModControl,Action:func(){ _=a.SaveActive() }})
	manager.AddShortcut(ui.Shortcut{Key:ui.KeyS,Mods:ui.ModSuper,Action:func(){ _=a.SaveActive() }})
	manager.AddShortcut(ui.Shortcut{Key:ui.KeyF5,Action:func(){ a.StartDebug("debug") }})
	return a
}

func (a *App) OpenProject(path string) error {
	absolute,err := filepath.Abs(path)
	if err != nil { return err }
	absolute,err = filepath.EvalSymlinks(absolute)
	if err != nil { return err }
	info,err := os.Stat(filepath.Join(absolute,"go.mod"))
	if err != nil || info.IsDir() { return fmt.Errorf("%s: go.mod не найден",absolute) }
	if err := a.saveAll(); err != nil { return err }
	a.StopDebug()
	if a.lsp!=nil { old:=a.lsp; a.lsp=nil; old.Close() }
	for path := range a.files { a.editorTabs.Close(path) }
	a.files=make(map[string]*openDocument)
	a.breakpoints=make(map[string]map[int]bool)
	a.project=newProjectTree(absolute)
	a.fileTree.Model=a.project
	a.fileTree.Expanded=make(map[string]bool)
	a.fileTree.SetExpanded(absolute,true)
	a.fileTree.ModelChanged()
	a.diagnosticByURI=make(map[string][]lspDiagnostic)
	a.diagnostics.rows=nil
	a.diagnosticsView.ModelChanged()
	a.status.SetText("Модуль: "+absolute)
	client,err := startLSP(a)
	if err != nil {
		a.output.AppendText("gopls: "+err.Error()+"\n")
		return nil // Editing and local Go colors are still available.
	}
	a.lsp=client
	return nil
}

func (a *App) openFile(path string) error {
	path,err := filepath.Abs(path)
	if err != nil { return err }
	if !within(a.project.root,path) { return fmt.Errorf("%s: файл вне модуля",path) }
	if file:=a.files[path]; file!=nil { a.editorTabs.Select(path); return nil }
	data,err := os.ReadFile(path)
	if err != nil { return err }
	document,err := editor.NewDocument(string(data))
	if err != nil { return err }
	view:=ui.NewCodeEditor(document)
	file:=&openDocument{path:path,document:document,view:view,lastText:string(data)}
	view.OnChange=func(change editor.Change) {
		file.dirty=true
		a.editorTabs.SetDirty(path,true)
		lines:=make(map[int]bool)
		for _,line:=range view.BreakpointLines() { lines[line]=true }
		a.breakpoints[path]=lines
		if a.debug!=nil && a.debug.configured { a.debug.setBreakpoints(path) }
		if a.lsp!=nil { a.lsp.change(file,change) } else { file.lastText=file.document.Text() }
	}
	view.OnBreakpointChange=func(line int,enabled bool){
		lines:=a.breakpoints[path]
		if lines==nil { lines=make(map[int]bool); a.breakpoints[path]=lines }
		if enabled { lines[line]=true } else { delete(lines,line) }
		if a.debug!=nil && a.debug.configured { a.debug.setBreakpoints(path) }
	}
	a.files[path]=file
	a.editorTabs.Open(ui.Tab{ID:path,Title:fileName(path),Content:view})
	file.shortcuts=append(file.shortcuts,a.manager.AddShortcut(ui.Shortcut{
		Key:ui.KeySpace,Mods:ui.ModControl,Scope:view,Action:func(){
			a.editorTabs.Select(path); a.Complete()
		},
	}))
	if a.lsp!=nil { a.lsp.open(file) }
	return nil
}

func (a *App) active() *openDocument { return a.files[a.editorTabs.SelectedID] }

func (a *App) SaveActive() error {
	if file:=a.active(); file!=nil { return a.save(file) }
	return nil
}
func (a *App) save(file *openDocument) error {
	if !file.dirty { return nil }
	if err := os.WriteFile(file.path,[]byte(file.document.Text()),0644); err != nil { return err }
	file.dirty=false
	a.editorTabs.SetDirty(file.path,false)
	a.status.SetText("Сохранено: "+fileName(file.path))
	return nil
}
func (a *App) saveAll() error {
	for _,file := range a.files { if err:=a.save(file); err!=nil { return err } }
	return nil
}

func (a *App) Complete() {
	file:=a.active()
	if file==nil || a.lsp==nil || !a.lsp.ready { a.status.SetText("gopls ещё не готов"); return }
	a.lsp.completion(file)
}

func (a *App) setDiagnostics(uri string,version *int,diagnostics []lspDiagnostic) {
	path:=uriPath(uri)
	if path=="" || !within(a.project.root,path) { return }
	if file:=a.files[path]; file!=nil && version!=nil && *version<file.lspVersion { return }
	a.diagnosticByURI[uri]=diagnostics
	a.diagnostics.rows=nil
	for uri,list := range a.diagnosticByURI {
		path:=uriPath(uri)
		for i,d := range list {
			a.diagnostics.rows=append(a.diagnostics.rows,diagnosticRow{
				id:fmt.Sprintf("%s:%d:%d:%d",path,d.Range.Start.Line,d.Range.Start.Character,i),
				path:path,line:d.Range.Start.Line,column:d.Range.Start.Character,
				message:strings.ReplaceAll(d.Message,"\n"," "),severity:d.Severity,
			})
		}
	}
	a.diagnosticsView.ModelChanged()
}

func (a *App) showFrame(row stackRow) {
	if row.path=="" || !within(a.project.root,row.path) { return }
	if err:=a.openFile(row.path); err!=nil { a.status.SetText(err.Error()); return }
	file:=a.files[row.path]
	line:=max(0,row.line-1)
	file.view.ScrollToLine(line)
	start,_,err:=file.document.LineRange(line)
	if err==nil { file.view.SetSelection(start,start) }
}

func (a *App) StartDebug(mode string) {
	if a.project.root=="" { a.status.SetText("Сначала откройте Go-модуль"); return }
	if err:=a.saveAll(); err!=nil { a.status.SetText(err.Error()); return }
	a.StopDebug()
	d,err:=startDebug(a,mode)
	if err!=nil { a.status.SetText("dlv: "+err.Error()); return }
	a.debug=d
	a.status.SetText("Запуск отладки: "+mode)
}
func (a *App) StopDebug() {
	if a.debug!=nil { old:=a.debug; a.debug=nil; old.Close() }
	a.stack.rows=nil
	a.variables.rows=nil
	a.stackView.ModelChanged()
	a.variablesView.ModelChanged()
}
func (a *App) debugControl(command string) {
	if a.debug==nil { a.status.SetText("Отладка не запущена"); return }
	a.debug.control(command)
}

func (a *App) OpenDropped(paths []string) {
	for _,path:=range paths {
		info,err:=os.Stat(path)
		if err!=nil { continue }
		if info.IsDir() { _=a.OpenProject(path); return }
		if a.project.root!="" && within(a.project.root,path) { _=a.openFile(path) }
	}
}

func (a *App) Close() {
	a.closed=true
	a.StopDebug()
	if a.lsp!=nil { old:=a.lsp; a.lsp=nil; old.Close() }
}
