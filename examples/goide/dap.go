package goide

import (
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"sort"
	"time"
)

type debugSession struct {
	app *App
	cmd *exec.Cmd
	listener net.Listener
	conn *protocolConn
	next int
	pending map[int]func(json.RawMessage,error)
	configured bool
	threadID int
	scopeGeneration uint64
}

func startDebug(app *App, mode string) (*debugSession,error) {
	listener, err := net.Listen("tcp","127.0.0.1:0")
	if err != nil { return nil,err }
	cmd := exec.Command("dlv","dap","--client-addr="+listener.Addr().String())
	cmd.Dir = app.project.root
	cmd.Stdout,cmd.Stderr = app.logWriter("dlv"),app.logWriter("dlv")
	if err := cmd.Start(); err != nil { _ = listener.Close(); return nil,err }
	d := &debugSession{app:app,cmd:cmd,listener:listener,pending:make(map[int]func(json.RawMessage,error))}
	go func(){ _ = cmd.Wait() }()
	go func() {
		if tcp,ok := listener.(*net.TCPListener); ok { _ = tcp.SetDeadline(time.Now().Add(15*time.Second)) }
		conn,err := listener.Accept()
		_ = listener.Close()
		app.manager.Post(func(){
			if app.debug != d || app.closed { if conn != nil { _ = conn.Close() }; return }
			if err != nil { app.status.SetText("dlv: "+err.Error()); d.Close(); app.debug=nil; return }
			d.conn = newProtocolConn(conn,conn,app.manager.Post,d.receive,func(err error){
				if app.debug == d && !app.closed { app.status.SetText("dlv: "+err.Error()) }
			})
			d.request("initialize",map[string]any{
				"clientID":"nativeForms-goide","adapterID":"go","pathFormat":"path",
				"linesStartAt1":true,"columnsStartAt1":true,
			},func(_ json.RawMessage,err error){
				if app.debug != d { return }
				if err != nil { app.status.SetText("dlv initialize: "+err.Error()); return }
				d.request("launch",map[string]any{
					"name":"Go "+mode,"type":"go","request":"launch",
					"mode":mode,"program":app.project.root,"cwd":app.project.root,
				},func(_ json.RawMessage,err error){
					if app.debug == d && err != nil { app.status.SetText("dlv launch: "+err.Error()) }
				})
			})
		})
	}()
	return d,nil
}

func (d *debugSession) Close() {
	if d.listener != nil { _ = d.listener.Close() }
	if d.conn != nil { d.conn.Close() }
	if d.cmd.Process != nil { _ = d.cmd.Process.Kill() }
}

func (d *debugSession) request(command string,args any,callback func(json.RawMessage,error)) {
	if d.conn == nil { return }
	d.next++
	id := d.next
	if callback != nil { d.pending[id]=callback }
	err := d.conn.send(map[string]any{"seq":id,"type":"request","command":command,"arguments":args})
	if err != nil {
		delete(d.pending,id)
		if callback != nil { callback(nil,err) }
	}
}

func (d *debugSession) receive(data json.RawMessage) {
	if d.app.debug != d { return }
	var envelope struct {
		Type string `json:"type"`
		Event string `json:"event"`
		Command string `json:"command"`
		RequestSeq int `json:"request_seq"`
		Success bool `json:"success"`
		Message string `json:"message"`
		Body json.RawMessage `json:"body"`
	}
	if json.Unmarshal(data,&envelope) != nil { return }
	switch envelope.Type {
	case "response":
		callback := d.pending[envelope.RequestSeq]
		delete(d.pending,envelope.RequestSeq)
		if callback != nil {
			if !envelope.Success { callback(nil,fmt.Errorf("%s: %s",envelope.Command,envelope.Message)) } else { callback(envelope.Body,nil) }
		}
	case "event":
		switch envelope.Event {
		case "initialized":
			paths := make([]string,0,len(d.app.breakpoints))
			for path := range d.app.breakpoints { paths=append(paths,path) }
			sort.Strings(paths)
			for _,path := range paths { d.setBreakpoints(path) }
			d.request("configurationDone",map[string]any{},func(_ json.RawMessage,err error){
				if d.app.debug == d && err != nil { d.app.status.SetText("dlv configuration: "+err.Error()) }
			})
			d.configured = true
		case "stopped":
			var body struct { Reason string `json:"reason"`; ThreadID int `json:"threadId"` }
			if json.Unmarshal(envelope.Body,&body) == nil {
				d.app.status.SetText("Остановка: "+body.Reason)
				d.fetchStack(body.ThreadID)
			}
		case "continued":
			d.scopeGeneration++
			d.threadID=0
			d.app.stack.rows=nil
			d.app.variables.rows=nil
			d.app.stackView.ModelChanged()
			d.app.variablesView.ModelChanged()
		case "output":
			var body struct { Output string `json:"output"` }
			if json.Unmarshal(envelope.Body,&body) == nil { d.app.output.AppendText(body.Output) }
		case "terminated","exited":
			d.app.status.SetText("Отладка завершена")
		}
	}
}

func (d *debugSession) setBreakpoints(path string) {
	lines := make([]int,0,len(d.app.breakpoints[path]))
	for line := range d.app.breakpoints[path] { lines=append(lines,line+1) }
	sort.Ints(lines)
	points := make([]map[string]int,0,len(lines))
	for _,line := range lines { points=append(points,map[string]int{"line":line}) }
	d.request("setBreakpoints",map[string]any{
		"source":map[string]any{"path":path,"name":fileName(path)},
		"breakpoints":points,
	},func(_ json.RawMessage,err error){
		if d.app.debug == d && err != nil { d.app.status.SetText("Breakpoint: "+err.Error()) }
	})
}

func (d *debugSession) control(command string) {
	if d.threadID == 0 { d.app.status.SetText("Нет остановленного потока"); return }
	d.request(command,map[string]any{"threadId":d.threadID},func(_ json.RawMessage,err error){
		if d.app.debug == d && err != nil { d.app.status.SetText("dlv: "+err.Error()) }
	})
}

func (d *debugSession) fetchStack(thread int) {
	if thread == 0 {
		d.request("threads",map[string]any{},func(result json.RawMessage,err error){
			if err != nil || d.app.debug != d { return }
			var body struct { Threads []struct { ID int `json:"id"` } `json:"threads"` }
			if json.Unmarshal(result,&body)==nil && len(body.Threads)>0 { d.fetchStack(body.Threads[0].ID) }
		})
		return
	}
	d.threadID=thread
	d.request("stackTrace",map[string]any{"threadId":thread,"startFrame":0,"levels":40},func(result json.RawMessage,err error){
		if err != nil || d.app.debug != d { return }
		var body struct {
			Frames []struct {
				ID int `json:"id"`
				Name string `json:"name"`
				Line int `json:"line"`
				Source struct { Path string `json:"path"` } `json:"source"`
			} `json:"stackFrames"`
		}
		if json.Unmarshal(result,&body)!=nil { return }
		d.app.stack.rows=nil
		for _,f := range body.Frames {
			d.app.stack.rows=append(d.app.stack.rows,stackRow{id:f.ID,name:f.Name,path:f.Source.Path,line:f.Line})
		}
		d.app.stackView.ModelChanged()
		if len(d.app.stack.rows)>0 { d.app.showFrame(d.app.stack.rows[0]); d.fetchScopes(d.app.stack.rows[0].id) }
	})
}

func (d *debugSession) fetchScopes(frameID int) {
	d.scopeGeneration++
	generation := d.scopeGeneration
	d.app.variables.rows=nil
	d.app.variablesView.ModelChanged()
	d.request("scopes",map[string]any{"frameId":frameID},func(result json.RawMessage,err error){
		if err != nil || d.app.debug != d || d.scopeGeneration != generation { return }
		var body struct { Scopes []struct {
			Name string `json:"name"`
			VariablesReference int `json:"variablesReference"`
		} `json:"scopes"` }
		if json.Unmarshal(result,&body)!=nil { return }
		for _,scope := range body.Scopes {
			if scope.VariablesReference == 0 { continue }
			name := scope.Name
			d.request("variables",map[string]any{"variablesReference":scope.VariablesReference},func(result json.RawMessage,err error){
				if err != nil || d.app.debug != d || d.scopeGeneration != generation { return }
				var vars struct { Variables []struct {
					Name string `json:"name"`
					Value string `json:"value"`
					Type string `json:"type"`
				} `json:"variables"` }
				if json.Unmarshal(result,&vars)!=nil { return }
				for _,v := range vars.Variables {
					d.app.variables.rows=append(d.app.variables.rows,variableRow{
						id:fmt.Sprint(len(d.app.variables.rows)),name:name+" · "+v.Name,value:v.Value,kind:v.Type})
				}
				d.app.variablesView.ModelChanged()
			})
		}
	})
}
