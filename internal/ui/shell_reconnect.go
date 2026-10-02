package ui

func (p *shellPane) reconnect() {
	s := p.workspace
	host, executable := p.host, p.executable
	p.stop()
	s.tabs.Remove(p.item)
	delete(s.panes, p.item)
	if host != nil {
		s.connectHost(*host)
	} else {
		s.openLocal(executable)
	}
}
