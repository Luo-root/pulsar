package worker

import "github.com/Luo-root/pulse/components/skill"

func (m *Manager) InitSkillLoader() error {
	sr := skill.NewSkillRegistry()
	m.skill = skill.NewSkillLoader(sr, m.registry)
	err := m.skill.LoadFromDir(m.config.Skill.Path)
	if err != nil {
		return err
	}
	return nil
}
