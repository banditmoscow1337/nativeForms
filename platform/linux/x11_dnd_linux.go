//go:build linux

package linux

import "time"

type x11DND struct {
	aware, enter, position, status, leave, drop, finished, selection, copyAction, types, uris uint32
	source, version                                                                           uint32
	offered, accepted                                                                         bool
}

func (w *X11Window) setupDND() error {
	d := &w.dnd
	for _, a := range []struct {
		name string
		dst  *uint32
	}{
		{"XdndAware", &d.aware}, {"XdndEnter", &d.enter}, {"XdndPosition", &d.position}, {"XdndStatus", &d.status}, {"XdndLeave", &d.leave}, {"XdndDrop", &d.drop}, {"XdndFinished", &d.finished}, {"XdndSelection", &d.selection}, {"XdndActionCopy", &d.copyAction}, {"XdndTypeList", &d.types}, {"text/uri-list", &d.uris},
	} {
		v, err := w.atom(a.name)
		if err != nil {
			return err
		}
		*a.dst = v
	}
	if w.options.OnFilesDropped == nil {
		return nil
	}
	data := make([]byte, 4)
	le.PutUint32(data, 5)
	return w.changeProperty(w.id, d.aware, 4, 32, data)
}
func (w *X11Window) clientMessage(destination, typ uint32, data [5]uint32) error {
	r := make([]byte, 44)
	r[0] = 25
	le.PutUint32(r[4:], destination)
	r[12], r[13] = 33, 32
	le.PutUint32(r[16:], destination)
	le.PutUint32(r[20:], typ)
	for i, v := range data {
		le.PutUint32(r[24+i*4:], v)
	}
	return w.sendPeer(r, destination)
}
func (w *X11Window) handleDND(event []byte) {
	if event[1] != 32 || le.Uint32(event[4:]) != w.id {
		return
	}
	d := &w.dnd
	typ, source := le.Uint32(event[8:]), le.Uint32(event[12:])
	switch typ {
	case d.enter:
		d.source, d.version = source, min(le.Uint32(event[16:])>>24, 5)
		d.offered, d.accepted = false, false
		if d.version < 3 || w.options.OnFilesDropped == nil {
			return
		}
		types := event[20:32]
		if le.Uint32(event[16:])&1 != 0 {
			p, err := w.readProperty(source, d.types, false, time.Now().Add(time.Second))
			if err != nil || p.typ != 4 || p.format != 32 {
				return
			}
			types = p.data
		}
		for i := 0; i+4 <= len(types); i += 4 {
			if le.Uint32(types[i:]) == d.uris {
				d.offered = true
			}
		}
	case d.position:
		if source != d.source {
			return
		}
		d.accepted = d.offered && le.Uint32(event[28:]) == d.copyAction
		flags, action := uint32(2), uint32(0)
		if d.accepted {
			flags |= 1
			action = d.copyAction
		}
		if err := w.clientMessage(source, d.status, [5]uint32{w.id, flags, 0, 0, action}); err != nil {
			w.clipboardError = err
		}
	case d.leave:
		if source == d.source {
			d.source = 0
			d.offered, d.accepted = false, false
		}
	case d.drop:
		if source != d.source {
			return
		}
		success := uint32(0)
		if d.accepted {
			data, err := w.convertSelection(d.selection, d.uris, le.Uint32(event[20:]))
			if err == nil {
				if files := waylandDroppedFiles(data); len(files) > 0 {
					w.options.OnFilesDropped(files)
					success = 1
				}
			}
		}
		action := uint32(0)
		if success != 0 {
			action = d.copyAction
		}
		if err := w.clientMessage(source, d.finished, [5]uint32{w.id, success, action}); err != nil {
			w.clipboardError = err
		}
		d.source = 0
		d.offered, d.accepted = false, false
	}
}
