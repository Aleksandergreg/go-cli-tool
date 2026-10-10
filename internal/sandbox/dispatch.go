package sandbox

import (
	"errors"
	"fmt"
)

func (s *Sandbox) run(context *executionContext, args []string, stdin string) (output string, err error) {
	if len(args) == 0 {
		return "", nil
	}
	context.dispatchSteps++
	if context.dispatchSteps > maxExecutionDispatchSteps {
		return "", fmt.Errorf("command dispatch limit of %d exceeded", maxExecutionDispatchSteps)
	}
	defer func() {
		if (err == nil || errors.Is(err, errFailureStatus)) && len(output) > maxCommandOutputBytes {
			output = ""
			err = commandOutputLimitError()
		}
	}()
	if len(context.scriptStack) > 0 {
		context.scriptSteps++
		if context.scriptSteps > maxScriptSteps {
			return "", fmt.Errorf("script command limit of %d exceeded", maxScriptSteps)
		}
	}
	if isExecutableScriptPath(args[0]) {
		context.commands = append(context.commands, "sh")
		return s.cmdExecutableScript(context, args, stdin)
	}
	context.commands = append(context.commands, args[0])
	switch args[0] {
	case "true":
		return "", nil
	case "false":
		return "", errFailureStatus
	case "pwd":
		return s.cmdPwd(args[1:])
	case "cd":
		return s.cmdCD(args[1:])
	case "ls":
		return s.cmdLS(args[1:])
	case "mkdir":
		return s.cmdMkdir(args[1:])
	case "touch":
		return s.cmdTouch(args[1:])
	case "cp":
		return s.cmdCopy(args[1:])
	case "mv":
		return s.cmdMove(args[1:])
	case "rm", "rmdir":
		return s.cmdRemove(args[0], args[1:])
	case "cat", "less":
		return s.cmdCat(args[1:], stdin)
	case "head":
		return s.cmdHeadTail(args[1:], stdin, true)
	case "tail":
		return s.cmdHeadTail(args[1:], stdin, false)
	case "history":
		return s.cmdHistory(args[1:])
	case "grep":
		return s.cmdGrep(args[1:], stdin)
	case "find":
		return s.cmdFind(context, args[1:])
	case "sh":
		return s.cmdSh(context, args[1:], stdin)
	case "chmod":
		return s.cmdChmod(args[1:])
	case "chown":
		return s.cmdChown(args[1:])
	case "ps":
		return s.cmdPS(args[1:])
	case "kill":
		return s.cmdKill(args[1:])
	case "tar":
		return s.cmdTar(args[1:])
	case "gzip", "gunzip":
		return s.cmdGzip(args[0], args[1:])
	case "echo":
		return cmdEcho(args[1:])
	case "printf":
		return cmdPrintf(args[1:])
	case "export":
		return s.cmdExport(args[1:])
	case "env":
		return s.cmdEnv(args[1:])
	case "sort":
		return s.cmdSort(args[1:], stdin)
	case "uniq":
		return s.cmdUniq(args[1:], stdin)
	case "wc":
		return s.cmdWC(args[1:], stdin)
	case "awk":
		return s.cmdAwk(args[1:], stdin)
	case "cut":
		return s.cmdCut(args[1:], stdin)
	case "sed":
		return s.cmdSed(args[1:], stdin)
	case "tr":
		return s.cmdTr(args[1:], stdin)
	case "du":
		return s.cmdDU(args[1:])
	case "stat":
		return s.cmdStat(args[1:])
	case "basename", "dirname":
		return cmdPathPart(args[0], args[1:])
	case "whoami":
		return s.Env["USER"] + "\n", nil
	case "clear":
		return "", nil
	case "vi":
		return "", fmt.Errorf("interactive commands are not supported inside find -exec")
	case "help", "man":
		return shellHelp(args[1:])
	default:
		return "", fmt.Errorf("command not available in this lab; type help to see supported commands")
	}
}
