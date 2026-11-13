package sandbox

import (
	"fmt"
)

type shellWord struct {
	value string
	glob  bool
}

type pipelineStage struct {
	args       []shellWord
	inputPath  string
	outputPath string
	append     bool
}

type commandLine struct {
	stages []pipelineStage
}

type expandedPipelineStage struct {
	args       []string
	inputPath  string
	outputPath string
	append     bool
}

type expandedCommandLine struct {
	stages []expandedPipelineStage
}

func parseCommandLine(tokens []token) (commandLine, error) {
	if len(tokens) == 0 {
		return commandLine{}, nil
	}
	parsed := commandLine{}
	stage := pipelineStage{}
	for i := 0; i < len(tokens); i++ {
		current := tokens[i]
		switch current.kind {
		case wordToken:
			stage.args = append(stage.args, shellWord{value: current.value, glob: current.glob})
		case pipeToken:
			if len(stage.args) == 0 {
				return commandLine{}, fmt.Errorf("unexpected pipe")
			}
			if len(parsed.stages) == maxPipelineStages-1 {
				return commandLine{}, fmt.Errorf("pipeline stage limit of %d exceeded", maxPipelineStages)
			}
			parsed.stages = append(parsed.stages, stage)
			stage = pipelineStage{}
		case redirectToken, appendToken, inputToken:
			if i+1 >= len(tokens) || tokens[i+1].kind != wordToken {
				return commandLine{}, fmt.Errorf("redirection requires a file")
			}
			i++
			file := tokens[i].value
			if current.kind == inputToken {
				if stage.inputPath != "" {
					return commandLine{}, fmt.Errorf("multiple input redirections")
				}
				stage.inputPath = file
			} else {
				if stage.outputPath != "" {
					return commandLine{}, fmt.Errorf("multiple output redirections")
				}
				stage.outputPath = file
				stage.append = current.kind == appendToken
			}
		}
	}
	if len(stage.args) == 0 {
		if len(parsed.stages) > 0 {
			return commandLine{}, fmt.Errorf("pipeline cannot end with a pipe")
		}
		return commandLine{}, fmt.Errorf("redirection requires a command")
	}
	if len(parsed.stages) == maxPipelineStages {
		return commandLine{}, fmt.Errorf("pipeline stage limit of %d exceeded", maxPipelineStages)
	}
	parsed.stages = append(parsed.stages, stage)
	return parsed, nil
}

func (s *Sandbox) expandCommandLine(parsed commandLine) (expandedCommandLine, error) {
	expanded := expandedCommandLine{stages: make([]expandedPipelineStage, 0, len(parsed.stages))}
	argumentCount := 0
	tokenBytes := 0
	consumeTokenBytes := func(value string) error {
		if len(value) > maxExpandedTokenBytes-tokenBytes {
			return fmt.Errorf("expanded command exceeds the %d KiB token limit", maxExpandedTokenBytes/1024)
		}
		tokenBytes += len(value)
		return nil
	}
	for _, stage := range parsed.stages {
		args := make([]string, 0, len(stage.args))
		for _, word := range stage.args {
			values := []string{word.value}
			if word.glob {
				if matches := s.FS.Glob(s.CWD, word.value); len(matches) > 0 {
					values = matches
				}
			}
			for _, value := range values {
				if argumentCount == maxExpandedArguments {
					return expandedCommandLine{}, fmt.Errorf("expanded command exceeds the %d-argument limit", maxExpandedArguments)
				}
				if err := consumeTokenBytes(value); err != nil {
					return expandedCommandLine{}, err
				}
				argumentCount++
				args = append(args, value)
			}
		}
		if err := consumeTokenBytes(stage.inputPath); err != nil {
			return expandedCommandLine{}, err
		}
		if err := consumeTokenBytes(stage.outputPath); err != nil {
			return expandedCommandLine{}, err
		}
		expanded.stages = append(expanded.stages, expandedPipelineStage{
			args:       args,
			inputPath:  stage.inputPath,
			outputPath: stage.outputPath,
			append:     stage.append,
		})
	}
	return expanded, nil
}
