package sbapp

func (m *Model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	sub := m.width / 5
	ns := m.width / 5
	ent := m.width / 5
	if sub < 24 {
		sub = 24
	}
	if ns < 24 {
		ns = 24
	}
	if ent < 24 {
		ent = 24
	}
	det := m.width - sub - ns - ent - 12
	if det < 40 {
		det = 40
	}

	height := m.height - 10
	if height < 8 {
		height = 8
	}

	if m.viewingMessage {
		detHalf := det / 3
		if detHalf < 30 {
			detHalf = 30
		}
		previewW := det - detHalf - 3
		if previewW < 30 {
			previewW = 30
		}
		m.detailList.SetSize(detHalf, height)
		m.messageViewport.Width = previewW
		m.messageViewport.Height = height - 2
	} else {
		m.detailList.SetSize(det, height)
		m.messageViewport.Width = 0
		m.messageViewport.Height = 0
	}

	m.subscriptionsList.SetSize(sub, height)
	m.namespacesList.SetSize(ns, height)
	m.entitiesList.SetSize(ent, height)
}
