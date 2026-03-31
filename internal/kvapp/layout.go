package kvapp

func (m *Model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	sub := m.width / 5
	vlt := m.width / 5
	sec := m.width / 5
	if sub < 24 {
		sub = 24
	}
	if vlt < 24 {
		vlt = 24
	}
	if sec < 24 {
		sec = 24
	}
	ver := m.width - sub - vlt - sec - 12
	if ver < 40 {
		ver = 40
	}

	height := m.height - 10
	if height < 8 {
		height = 8
	}

	m.subscriptionsList.SetSize(sub, height)
	m.vaultsList.SetSize(vlt, height)
	m.secretsList.SetSize(sec, height)
	m.versionsList.SetSize(ver, height)
}
