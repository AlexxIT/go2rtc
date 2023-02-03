package streams

import (
	"os"
	"strconv"

	"github.com/AlexxIT/go2rtc/cmd/app"
	"gopkg.in/yaml.v3"
)

const EMPTY_YAML = `
streams:
`

func setDict(root *yaml.Node, path []string, value yaml.Node) error {
	if len(path) == 0 {
		*root = value
		return nil
	}
	key := path[0]
	rest := path[1:]
	switch root.Kind {
	case yaml.DocumentNode:
		setDict(root.Content[0], path, value)
	case yaml.MappingNode:
		for i := 0; i < len(root.Content); i += 2 {
			if root.Content[i].Value == key {
				setDict(root.Content[i+1], rest, value)
				return nil
			}
		}
	case yaml.SequenceNode:
		index, err := strconv.Atoi(key)
		if err != nil {
			return err
		}
		setDict(root.Content[index], rest, value)
	}
	return nil
}

// load yaml file from app.ConfigPath
func load(cfg *yaml.Node) error {
	data, err := os.ReadFile(app.ConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			//not exists use default
			data = []byte(EMPTY_YAML)
		} else {
			return err
		}
	}
	if err = yaml.Unmarshal(data, cfg); err != nil {
		return err
	}
	return nil
}

// save yaml file into app.ConfigPath
func save(cfg yaml.Node) error {
	data, err := yaml.Marshal(cfg.Content[0])
	if err != nil {
		return err
	}
	err = os.WriteFile(app.ConfigPath, data, 0644)
	return err
}

// storeStreams reads config file and update
// its content with streams in memory
// storeStreams return error if cannot read or save config file
func storeStreams() error {
	var cfg yaml.Node
	err := load(&cfg)
	if err != nil {
		return err
	}
	path := []string{"streams"}

	streamsNode := []*yaml.Node{}
	for name, s := range streams {
		nVal := yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: name,
		}
		streamsNode = append(streamsNode, &nVal)
		if len(s.producers) > 1 {
			contents := []*yaml.Node{}
			for _, p := range s.producers {
				pVal := yaml.Node{
					Kind:  yaml.ScalarNode,
					Value: p.url,
				}
				contents = append(contents, &pVal)
			}
			sVal := yaml.Node{
				Kind:    yaml.SequenceNode,
				Content: contents,
			}
			streamsNode = append(streamsNode, &sVal)
		} else if len(s.producers) == 1 {
			p := s.producers[0]
			pVal := yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: p.url,
			}
			streamsNode = append(streamsNode, &pVal)
		}
	}

	streamVal := yaml.Node{
		Kind:    yaml.MappingNode,
		Content: streamsNode,
	}
	if err := setDict(&cfg, path, streamVal); err != nil {
		return err
	}
	return save(cfg)
}
