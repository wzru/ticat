package display

import (
	"sort"
	"strings"

	"github.com/pingcap/ticat/pkg/cli/core"
)

func DumpFlow(
	cc *core.Cli,
	env *core.Env,
	flow *core.ParsedCmds,
	fromCmdIdx int,
	args *DumpFlowArgs,
	envOpCmds []core.EnvOpCmd) {

	DumpFlowEx(cc, env, flow, fromCmdIdx, args, nil, false, envOpCmds)
}

// 'parsedGlobalEnv' + env in 'flow' = all env
func DumpFlowEx(
	cc *core.Cli,
	env *core.Env,
	flow *core.ParsedCmds,
	fromCmdIdx int,
	args *DumpFlowArgs,
	executedFlow *core.ExecutedFlow,
	running bool,
	envOpCmds []core.EnvOpCmd) {

	if len(flow.Cmds) == 0 {
		return
	}

	// The env will be modified during dumping (so it could show the real value)
	// so we need to clone the env to protect it
	env = env.Clone()

	writtenKeys := FlowWrittenKeys{}

	if executedFlow != nil {
	} else {
		PrintTipTitle(cc.Screen, env, "flow executing description:")
	}

	cc.Screen.Print(ColorFlowing("--->>>", env) + "\n")
	ok := dumpFlow(cc, env, envOpCmds, flow, fromCmdIdx, args, executedFlow, running,
		writtenKeys, args.MaxDepth, args.MaxTrivial, 0)
	if ok {
		cc.Screen.Print(ColorFlowing("<<<---", env) + "\n")
	}
}

func dumpFlow(
	cc *core.Cli,
	env *core.Env,
	envOpCmds []core.EnvOpCmd,
	flow *core.ParsedCmds,
	fromCmdIdx int,
	args *DumpFlowArgs,
	executedFlow *core.ExecutedFlow,
	running bool,
	writtenKeys FlowWrittenKeys,
	maxDepth int,
	maxTrivial int,
	indentAdjust int) bool {

	metFlows := map[string]bool{}
	for i, cmd := range flow.Cmds[fromCmdIdx:] {
		if cmd.IsEmpty() {
			continue
		}
		var executedCmd *core.ExecutedCmd
		if executedFlow != nil {
			if i < len(executedFlow.Cmds) {
				executedCmd = executedFlow.GetCmd(i)
			} else {
				return false
			}
		}
		ok := dumpFlowCmd(cc, cc.Screen, env, envOpCmds, flow, fromCmdIdx+i, args, executedCmd, running,
			maxDepth, maxTrivial, indentAdjust, metFlows, writtenKeys)
		if !ok {
			return false
		}
	}
	return true
}

func dumpFlowCmd(
	cc *core.Cli,
	screen core.Screen,
	env *core.Env,
	envOpCmds []core.EnvOpCmd,
	flow *core.ParsedCmds,
	currCmdIdx int,
	args *DumpFlowArgs,
	executedCmd *core.ExecutedCmd,
	running bool,
	maxDepth int,
	maxTrivial int,
	indentAdjust int,
	metFlows map[string]bool,
	writtenKeys FlowWrittenKeys) bool {

	// TODO: too complicated with executedCmd

	parsedCmd := flow.Cmds[currCmdIdx]
	cmd := parsedCmd.Last().Matched.Cmd
	if cmd == nil {
		return true
	}

	trivialMark := env.GetRaw("strs.trivial-mark")

	sep := cmd.Strs.PathSep
	envOpSep := " " + cmd.Strs.EnvOpSep + " "

	prt := func(indentLvl int, msg string) {
		indentLvl += indentAdjust
		padding := rpt(" ", args.IndentSize*indentLvl)
		msg = autoPadNewLine(padding, msg)
		screen.Print(padding + msg + "\n")
	}

	cic := cmd.Cmd()
	if cic == nil {
		return true
	}

	trivialDelta := cmd.Trivial() + parsedCmd.TrivialLvl

	cmdEnv, argv := parsedCmd.ApplyMappingGenEnvAndArgv(env, cc.Cmds.Strs.EnvValDelAllMark, sep)
	sysArgv := cmdEnv.GetSysArgv(cmd.Path(), sep)

	if (executedCmd != nil && !executedCmd.Succeeded) || (maxTrivial > 0 && maxDepth > 0) {
		cmdId := strings.Join(parsedCmd.Path(), sep)
		var name string
		if args.Skeleton {
			name = cmdId
		} else {
			name = parsedCmd.DisplayPath(sep, true)
		}
		if sysArgv.IsDelay() {
			name = ColorCmdDelay("["+name+"]", env)
		} else {
			name = ColorCmd("["+name+"]", env)
		}
		if (executedCmd == nil || executedCmd.Succeeded) && maxTrivial == 1 && trivialDelta > 0 &&
			(cic.Type() == core.CmdTypeFlow || cic.Type() == core.CmdTypeFileNFlow) {
			name += ColorProp(trivialMark, env)
		}

		if sysArgv.IsDelay() {
			name += ColorCmdDelay(" (schedule in ", env) + sysArgv.GetDelayStr() + ColorCmdDelay(")", env)
		}

		if executedCmd != nil {
			if executedCmd.Cmd != cmdId {
				// TODO: better display
				name += ColorSymbol(" - ", env) + ColorError("flow not matched, origin cmd: ", env) +
					ColorCmd("["+executedCmd.Cmd+"]", env)
				prt(0, name)
				return false
			}
			if executedCmd.Unexecuted {
				name += ColorSymbol(" - ", env) + ColorExplain("un-run", env)
			} else if executedCmd.Succeeded {
				name += ColorSymbol(" - ", env) + ColorCmdDone("OK", env)
			} else if running {
				name += ColorSymbol(" - ", env) + ColorError("not-done", env)
			} else {
				name += ColorSymbol(" - ", env) + ColorError("ERR", env)
			}
		}

		prt(0, name)

		if len(cic.Help()) != 0 {
			if !args.Skeleton {
				prt(1, " "+ColorHelp("'"+cic.Help()+"'", env))
			} else {
				prt(1, ColorHelp("'"+cic.Help()+"'", env))
			}
		}

		if executedCmd != nil {
			if executedCmd.Unexecuted {
				return true
			}
			if !args.Skeleton {
				if len(executedCmd.Err) != 0 {
					prt(1, ColorError("- error:", env))
				}
				for _, line := range executedCmd.Err {
					prt(2, ColorError(line, env))
				}
			} else {
				if len(executedCmd.Err) != 0 {
					prt(0, "  "+ColorError(" - error:", env))
				}
				for _, line := range executedCmd.Err {
					prt(1, " "+ColorError(line, env))
				}
			}
		}
	}

	// TODO: this is slow
	originEnv := env.Clone()

	if (executedCmd != nil && !executedCmd.Succeeded) || (maxTrivial > 0 && maxDepth > 0) {
		if !args.Skeleton {
			args := cic.Args()
			arg2env := cic.GetArg2Env()
			argLines := DumpEffectedArgs(originEnv, arg2env, &args, argv, writtenKeys)
			if len(argLines) != 0 {
				prt(1, ColorProp("- args:", env))
			}
			for _, line := range argLines {
				prt(2, line)
			}
			//sysArgv := cmdEnv.GetSysArgv(cmd.Path(), sep)
			//for _, line := range DumpSysArgs(env, sysArgv, true) {
			//	prt(2, line)
			//}
		} else {
			for name, val := range argv {
				if !val.Provided {
					continue
				}
				prt(1, " "+ColorArg(name, env)+ColorSymbol(" = ", env)+val.Raw)
			}
			//sysArgv := cmdEnv.GetSysArgv(cmd.Path(), sep)
			//for _, line := range DumpSysArgs(env, sysArgv, true) {
			//	prt(1, " "+line)
			//}
		}
	}

	trivial := maxTrivial - trivialDelta

	// Folding if too trivial or too deep
	if (executedCmd == nil || executedCmd.Succeeded) &&
		(trivial <= 0 || maxDepth <= 0) {

		core.TryExeEnvOpCmds(argv, cc, cmdEnv, flow, currCmdIdx, envOpCmds, nil,
			"failed to execute env-op cmd in flow desc")

		// This is for render checking, even it's folded
		subFlow, rendered := cic.Flow(argv, cmdEnv, true)
		if rendered {
			parsedFlow := cc.Parser.Parse(cc.Cmds, cc.EnvAbbrs, subFlow...)
			err := parsedFlow.FirstErr()
			if err != nil {
				panic(err.Error)
			}
			parsedFlow.GlobalEnv.WriteNotArgTo(env, cc.Cmds.Strs.EnvValDelAllMark)
			var executedFlow *core.ExecutedFlow
			if executedCmd != nil {
				executedFlow = executedCmd.SubFlow
			}
			return dumpFlow(cc, env, envOpCmds, parsedFlow, 0, args, executedFlow, running,
				writtenKeys, maxDepth-1, trivial, indentAdjust+2)
		}
		return executedCmd == nil || executedCmd.Succeeded
	}

	if !args.Skeleton {
		keys, kvs := dumpFlowEnv(cc, originEnv, flow.GlobalEnv, parsedCmd, cmd, argv, writtenKeys)
		if len(keys) != 0 {
			prt(1, ColorProp("- env-values:", env))
		}
		for _, k := range keys {
			v := kvs[k]
			prt(2, ColorKey(k, env)+ColorSymbol(" = ", env)+mayQuoteStr(v.Val)+" "+v.Source+"")
		}
	}
	writtenKeys.AddCmd(argv, env, cic)

	if !args.Skeleton {
		envOps := cic.EnvOps()
		envOpKeys, origins, _ := envOps.RenderedEnvKeys(argv, cmdEnv, cic, false)
		if len(envOpKeys) != 0 {
			prt(1, ColorProp("- env-ops:", env))
		}
		for i, k := range envOpKeys {
			prt(2, ColorKey(k, env)+ColorSymbol(" = ", env)+
				dumpEnvOps(envOps.Ops(origins[i]), envOpSep)+dumpIsAutoTimerKey(env, cic, k))
		}
	}

	if !args.Simple && !args.Skeleton {
		line := string(cic.Type())
		if cic.IsQuiet() {
			line += " quiet"
		}
		if cic.IsPriority() {
			line += " priority"
		}
		prt(1, ColorProp("- cmd-type:", env))
		prt(2, line)

		if len(cmd.Source()) != 0 && !strings.HasPrefix(cic.CmdLine(), cmd.Source()) {
			prt(1, ColorProp("- from:", env))
			prt(2, cmd.Source())
		}
	}

	if (len(cic.CmdLine()) != 0 || len(cic.FlowStrs()) != 0) &&
		cic.Type() != core.CmdTypeNormal && cic.Type() != core.CmdTypePower {
		metFlow := false
		if cic.Type() == core.CmdTypeFlow || cic.Type() == core.CmdTypeFileNFlow {
			flowStrs, _ := cic.RenderedFlowStrs(argv, cmdEnv, true)
			flowStr := strings.Join(flowStrs, " ")
			metFlow = metFlows[flowStr]
			if (executedCmd == nil || executedCmd.Succeeded) && metFlow {
				if !args.Skeleton {
					prt(1, ColorProp("- flow (duplicated):", env))
				} else {
					prt(1, ColorProp("(duplicated sub flow)", env))
				}
			} else {
				metFlows[flowStr] = true
				if (executedCmd == nil || executedCmd.Succeeded) && maxDepth <= 1 {
					if !args.Skeleton {
						prt(1, ColorProp("- flow (folded):", env))
					} else {
						prt(1, ColorProp("(folded flow)", env))
					}
				} else {
					if !args.Skeleton {
						prt(1, ColorProp("- flow:", env))
					}
				}
			}
			if !args.Skeleton {
				for _, flowStr := range flowStrs {
					prt(2, ColorFlow(flowStr, env))
				}
			}
		} else if !args.Simple && !args.Skeleton {
			if cic.Type() == core.CmdTypeEmptyDir {
				prt(1, ColorProp("- dir:", env))
				prt(2, cic.CmdLine())
			} else {
				prt(1, ColorProp("- executable:", env))
				prt(2, cic.CmdLine())
			}
			if len(cic.MetaFile()) != 0 {
				prt(1, ColorProp("- meta:", env))
				prt(2, cic.MetaFile())
			}
		}

		if cic.Type() == core.CmdTypeFlow || cic.Type() == core.CmdTypeFileNFlow {
			subFlow, rendered := cic.Flow(argv, cmdEnv, true)
			if rendered && len(subFlow) != 0 {
				if !(metFlow && (executedCmd == nil || executedCmd.Succeeded)) {
					if (executedCmd != nil && !executedCmd.Succeeded) || maxDepth > 1 {
						prt(2, ColorFlowing("--->>>", env))
					}
					parsedFlow := cc.Parser.Parse(cc.Cmds, cc.EnvAbbrs, subFlow...)
					err := parsedFlow.FirstErr()
					if err != nil {
						panic(err.Error)
					}

					// TODO: need it or not ?
					parsedFlow.GlobalEnv.WriteNotArgTo(env, cc.Cmds.Strs.EnvValDelAllMark)

					var executedFlow *core.ExecutedFlow
					if executedCmd != nil {
						executedFlow = executedCmd.SubFlow
					}
					newMaxDepth := maxDepth
					if executedCmd == nil || executedCmd.Succeeded {
						newMaxDepth -= 1
					}
					ok := dumpFlow(cc, env, envOpCmds, parsedFlow, 0, args, executedFlow, running,
						writtenKeys, newMaxDepth, trivial, indentAdjust+2)
					if !ok {
						return false
					}
					exeMark := ""
					if cic.Type() == core.CmdTypeFileNFlow {
						exeMark = ColorCmd(" +", env)
					}
					if (executedCmd == nil || executedCmd.Succeeded) && maxDepth > 1 {
						prt(2, ColorFlowing("<<<---", env)+exeMark)
					}
				}
			}
		}
	}

	core.TryExeEnvOpCmds(argv, cc, cmdEnv, flow, currCmdIdx, envOpCmds, nil,
		"failed to execute env-op cmd in flow desc")
	return executedCmd == nil || executedCmd.Succeeded
}

type flowEnvVal struct {
	Val    string
	Source string
}

func dumpFlowEnv(
	cc *core.Cli,
	env *core.Env,
	parsedGlobalEnv core.ParsedEnv,
	parsedCmd core.ParsedCmd,
	cmd *core.CmdTree,
	argv core.ArgVals,
	writtenKeys FlowWrittenKeys) (keys []string, kvs map[string]flowEnvVal) {

	kvs = map[string]flowEnvVal{}
	cic := cmd.Cmd()

	tempEnv := core.NewEnv()
	parsedGlobalEnv.WriteNotArgTo(tempEnv, cc.Cmds.Strs.EnvValDelAllMark)
	cmdEssEnv := parsedCmd.GenCmdEnv(tempEnv, cc.Cmds.Strs.EnvValDelAllMark)
	val2env := cic.GetVal2Env()
	for _, k := range val2env.EnvKeys() {
		kvs[k] = flowEnvVal{val2env.Val(k), ColorSymbol("<- mod", env)}
	}

	flatten := cmdEssEnv.Flatten(true, nil, true)
	for k, v := range flatten {
		kvs[k] = flowEnvVal{v, ColorSymbol("<- flow", env)}
	}

	arg2env := cic.GetArg2Env()
	for name, val := range argv {
		if !val.Provided && len(val.Raw) == 0 {
			continue
		}
		key, hasMapping := arg2env.GetEnvKey(name)
		if !hasMapping {
			continue
		}
		_, inEnv := env.GetEx(key)
		if !val.Provided && inEnv {
			continue
		}
		if writtenKeys[key] {
			continue
		}
		kvs[key] = flowEnvVal{val.Raw, ColorSymbol("<- arg", env) +
			ColorArg(" '"+name+"'", env)}
	}

	for k, _ := range kvs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return
}

type DumpFlowArgs struct {
	Simple     bool
	Skeleton   bool
	IndentSize int
	MaxDepth   int
	MaxTrivial int
}

func NewDumpFlowArgs() *DumpFlowArgs {
	return &DumpFlowArgs{false, false, 4, 32, 1}
}

func (self *DumpFlowArgs) SetSimple() *DumpFlowArgs {
	self.Simple = true
	return self
}

func (self *DumpFlowArgs) SetMaxDepth(val int) *DumpFlowArgs {
	self.MaxDepth = val
	return self
}

func (self *DumpFlowArgs) SetMaxTrivial(val int) *DumpFlowArgs {
	self.MaxTrivial = val
	return self
}

func (self *DumpFlowArgs) SetSkeleton() *DumpFlowArgs {
	self.Simple = true
	self.Skeleton = true
	return self
}

type FlowWrittenKeys map[string]bool

func (self FlowWrittenKeys) AddCmd(argv core.ArgVals, env *core.Env, cic *core.Cmd) {
	if cic == nil {
		return
	}
	ops := cic.EnvOps()
	keys, _, _ := ops.RenderedEnvKeys(argv, env, cic, false)
	for _, k := range keys {
		// If is read-op, then the key must exists, so no need to check the op flags
		self[k] = true
	}
}
