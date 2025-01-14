package builtin

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pingcap/ticat/pkg/cli/core"
	"github.com/pingcap/ticat/pkg/cli/display"
	"github.com/pingcap/ticat/pkg/proto/flow_file"
	"github.com/pingcap/ticat/pkg/proto/mod_meta"
	"github.com/pingcap/ticat/pkg/utils"
)

func ListFlows(
	argv core.ArgVals,
	cc *core.Cli,
	env *core.Env,
	flow *core.ParsedCmds,
	currCmdIdx int) (int, bool) {

	flowExt := env.GetRaw("strs.flow-ext")
	root := getFlowRoot(env, flow.Cmds[currCmdIdx])

	screen := display.NewCacheScreen()
	findStrs := getFindStrsFromArgvAndFlow(flow, currCmdIdx, argv)

	filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if path == root {
			return nil
		}
		if !strings.HasSuffix(path, flowExt) {
			return nil
		}

		cmdPath := getCmdPath(path, flowExt, flow.Cmds[currCmdIdx])
		flowStrs, help, abbrsStr := flow_file.LoadFlowFile(path)
		flowStr := strings.Join(flowStrs, " ")

		matched := true
		for _, findStr := range findStrs {
			if len(findStr) == 0 {
				continue
			}
			if strings.Index(cmdPath, findStr) < 0 &&
				strings.Index(help, findStr) < 0 &&
				strings.Index(abbrsStr, findStr) < 0 &&
				strings.Index(flowStr, findStr) < 0 {
				matched = false
				break
			}
		}
		if !matched {
			return nil
		}

		screen.Print(fmt.Sprintf(display.ColorCmd("[%s]\n", env), cmdPath))
		if len(help) != 0 {
			screen.Print("     " + display.ColorHelp("'"+help+"'", env) + "\n")
		}
		if len(abbrsStr) != 0 {
			screen.Print("    " + display.ColorProp("- abbrs:", env) + "\n")
			screen.Print(fmt.Sprintf("        %s\n", abbrsStr))
		}
		screen.Print("    " + display.ColorProp("- flow:", env) + "\n")
		for _, flowStr := range flowStrs {
			screen.Print("        " + display.ColorFlow(flowStr, env) + "\n")
		}
		screen.Print("    " + display.ColorProp("- executable:", env) + "\n")
		screen.Print(fmt.Sprintf("        %s\n", path))
		return nil
	})

	if screen.OutputNum() > 0 {
		display.PrintTipTitle(cc.Screen, env,
			"all saved flows: (flows from added repos are not included)")
	} else {
		display.PrintTipTitle(cc.Screen, env,
			"there is no saved flows yet, save flow by:",
			"",
			display.SuggestFlowAdd(env))
	}
	screen.WriteTo(cc.Screen)
	if display.TooMuchOutput(env, screen) {
		display.PrintTipTitle(cc.Screen, env,
			"filter flows by keywords if there are too many:",
			"",
			display.SuggestFlowsFilter(env))
	}
	return currCmdIdx, true
}

func RemoveFlow(
	argv core.ArgVals,
	cc *core.Cli,
	env *core.Env,
	flow *core.ParsedCmds,
	currCmdIdx int) (int, bool) {

	cmdPath, filePath := getFlowCmdPath(flow, currCmdIdx, true, argv, cc, env, true, "cmd-path")
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		panic(core.NewCmdError(flow.Cmds[currCmdIdx],
			fmt.Sprintf("path '%s' does not exist", filePath)))
	}
	err = os.Remove(filePath)
	if err != nil {
		panic(core.NewCmdError(flow.Cmds[currCmdIdx],
			fmt.Sprintf("remove flow file '%s' failed: %v", filePath, err)))
	}

	display.PrintTipTitle(cc.Screen, env,
		"flow '"+cmdPath+"' is removed")
	cc.Screen.Print(fmt.Sprintf(display.ColorCmd("[%s]", env)+
		display.ColorDisabled(" (removed)", env)+"\n", cmdPath))
	cc.Screen.Print(fmt.Sprintf("    %s\n", filePath))
	return currCmdIdx, true
}

func RemoveAllFlows(
	argv core.ArgVals,
	cc *core.Cli,
	env *core.Env,
	flow *core.ParsedCmds,
	currCmdIdx int) (int, bool) {

	flowExt := env.GetRaw("strs.flow-ext")
	root := getFlowRoot(env, flow.Cmds[currCmdIdx])
	screen := display.NewCacheScreen()

	filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if path != root && strings.HasSuffix(path, flowExt) {
			err = os.Remove(path)
			if err != nil {
				panic(core.NewCmdError(flow.Cmds[currCmdIdx],
					fmt.Sprintf("remove flow file '%s' failed: %v", path, err)))
			}
			cmdPath := getCmdPath(path, flowExt, flow.Cmds[currCmdIdx])
			screen.Print(fmt.Sprintf(display.ColorCmd("[%s]", env)+
				display.ColorDisabled(" (removed)", env)+"\n", cmdPath))
			screen.Print(fmt.Sprintf("    %s\n", path))
		}
		return nil
	})

	if screen.OutputNum() > 0 {
		display.PrintTipTitle(cc.Screen, env, "all saved flows are removed")
	} else {
		display.PrintTipTitle(cc.Screen, env,
			"there is no saved flows yet, nothing to do.",
			"",
			"save flow by:",
			"",
			display.SuggestFlowAdd(env))
	}
	screen.WriteTo(cc.Screen)
	return currCmdIdx, true
}

func SaveFlow(
	argv core.ArgVals,
	cc *core.Cli,
	env *core.Env,
	flow *core.ParsedCmds,
	currCmdIdx int) (int, bool) {

	cmdPath, filePath := getFlowCmdPath(flow, currCmdIdx, false, argv, cc, env, false, "to-cmd-path")
	screen := display.NewCacheScreen()

	_, err := os.Stat(filePath)
	if !os.IsNotExist(err) {
		if !env.GetBool("sys.interact") {
			panic(core.NewCmdError(flow.Cmds[currCmdIdx],
				fmt.Sprintf("path '%s' exists", filePath)))
		} else {
			cc.Screen.Print(fmt.Sprintf(display.ColorTip("[confirm]", env)+
				" flow file of '%s' exists, "+
				"type "+display.ColorWarn("'y'", env)+" and press enter to "+
				display.ColorWarn("overwrite:", env)+"\n", cmdPath))
			utils.UserConfirm()
		}
	}

	w := bytes.NewBuffer(nil)
	flow.RemoveLeadingCmds(1)

	if !checkAndConfirmIfFlowHasParseError(cc.Screen, flow, env) {
		return currCmdIdx, false
	}

	trivialMark := env.GetRaw("strs.trivial-mark")

	// TODO: wrap line if too long
	saveFlow(w, flow, currCmdIdx, cc.Cmds.Strs.PathSep, trivialMark, env)
	flowStr := w.String()

	screen.Print(fmt.Sprintf(display.ColorCmd("[%s]", env)+"\n", cmdPath))
	screen.Print("    " + display.ColorProp("- flow:", env) + "\n")
	screen.Print("        " + display.ColorFlow(flowStr, env) + "\n")
	screen.Print("    " + display.ColorProp("- executable:", env) + "\n")
	screen.Print(fmt.Sprintf("        %s\n", filePath))

	dirPath := filepath.Dir(filePath)
	os.MkdirAll(dirPath, os.ModePerm)

	flow_file.SaveFlowFile(filePath, []string{flowStr}, "", "")

	display.PrintTipTitle(cc.Screen, env,
		"flow '"+cmdPath+"' is saved, can be used as a command")
	screen.WriteTo(cc.Screen)
	return clearFlow(flow)
}

func SetFlowHelpStr(
	argv core.ArgVals,
	cc *core.Cli,
	env *core.Env,
	flow *core.ParsedCmds,
	currCmdIdx int) (int, bool) {

	help := argv.GetRaw("help-str")
	cmdPath, filePath := getFlowCmdPath(flow, currCmdIdx, true, argv, cc, env, true, "cmd-path")
	flowStrs, oldHelp, abbrsStr := flow_file.LoadFlowFile(filePath)
	flow_file.SaveFlowFile(filePath, flowStrs, help, abbrsStr)

	display.PrintTipTitle(cc.Screen, env,
		"help string of flow '"+cmdPath+"' is saved")

	cc.Screen.Print(display.ColorCmd(fmt.Sprintf("[%s]", cmdPath), env) + "\n")
	cc.Screen.Print("     " + display.ColorHelp("'"+help+"'", env) + "\n")
	cc.Screen.Print("    " + display.ColorProp("- flow:", env) + "\n")
	for _, flowStr := range flowStrs {
		cc.Screen.Print("        " + display.ColorFlow(flowStr, env) + "\n")
	}
	cc.Screen.Print("    " + display.ColorProp("- executable:", env) + "\n")
	cc.Screen.Print(fmt.Sprintf("        %s\n", filePath))
	if len(oldHelp) != 0 {
		cc.Screen.Print("    " + display.ColorProp("- old-help:", env) + "\n")
		cc.Screen.Print("       " + display.ColorHelp("'"+help+"'", env) + "\n")
	}
	return currCmdIdx, true
}

func LoadFlows(
	argv core.ArgVals,
	cc *core.Cli,
	env *core.Env,
	flow *core.ParsedCmds,
	currCmdIdx int) (int, bool) {

	assertNotTailMode(flow, currCmdIdx)
	root := getFlowRoot(env, flow.Cmds[currCmdIdx])
	loadFlowsFromDir(flow, currCmdIdx, root, cc, env, root)
	return currCmdIdx, true
}

func LoadFlowsFromDir(
	argv core.ArgVals,
	cc *core.Cli,
	env *core.Env,
	flow *core.ParsedCmds,
	currCmdIdx int) (int, bool) {

	path := tailModeCallArg(flow, currCmdIdx, argv, "path")
	loadFlowsFromDir(flow, currCmdIdx, path, cc, env, path)
	display.PrintTipTitle(cc.Screen, env,
		"flows from '"+path+"' is loaded")
	return currCmdIdx, true
}

func loadFlowsFromDir(
	flow *core.ParsedCmds,
	currCmdIdx int,
	root string,
	cc *core.Cli,
	env *core.Env,
	source string) bool {

	if len(root) > 0 && root[len(root)-1] == filepath.Separator {
		root = root[:len(root)-1]
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return true
		}
		panic(core.NewCmdError(flow.Cmds[currCmdIdx],
			fmt.Sprintf("access flows dir '%s' failed: %v", root, err)))
	}
	if !info.IsDir() {
		panic(core.NewCmdError(flow.Cmds[currCmdIdx],
			fmt.Sprintf("flows dir '%s' is not dir", root)))
	}

	flowExt := env.GetRaw("strs.flow-ext")
	envPathSep := env.GetRaw("strs.env-path-sep")
	panicRecover := env.GetBool("sys.panic.recover")

	filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, flowExt) {
			return nil
		}
		cmdPath := filepath.Base(path[0 : len(path)-len(flowExt)])
		cmdPaths := strings.Split(cmdPath, cc.Cmds.Strs.PathSep)
		mod_meta.RegMod(cc, path, "", false, true, cmdPaths, cc.Cmds.Strs.AbbrsSep, envPathSep, source, panicRecover)
		return nil
	})
	return true
}

func saveFlow(w io.Writer, flow *core.ParsedCmds, currCmdIdx int, cmdPathSep string, trivialMark string, env *core.Env) {
	envPathSep := env.GetRaw("strs.env-path-sep")
	bracketLeft := env.GetRaw("strs.env-bracket-left")
	bracketRight := env.GetRaw("strs.env-bracket-right")
	envKeyValSep := env.GetRaw("strs.env-kv-sep")
	seqSep := env.GetRaw("strs.seq-sep")
	if len(envPathSep) == 0 || len(bracketLeft) == 0 || len(bracketRight) == 0 ||
		len(envKeyValSep) == 0 || len(seqSep) == 0 {
		panic(core.NewCmdError(flow.Cmds[currCmdIdx], "some predefined strs not found"))
	}

	for i, cmd := range flow.Cmds {
		if len(flow.Cmds) > 1 {
			if i == 0 {
				if flow.GlobalCmdIdx < 0 {
					fmt.Fprint(w, seqSep+" ")
				}
			} else {
				fmt.Fprint(w, " "+seqSep+" ")
			}
		}

		if cmd.ParseResult.Error != nil {
			fmt.Fprint(w, strings.Join(cmd.ParseResult.Input, " "))
			continue
		}

		var path []string
		var lastSegHasNoCmd bool
		var cmdHasEnv bool

		for i := 0; i < cmd.TrivialLvl; i++ {
			fmt.Fprint(w, trivialMark)
		}

		for j, seg := range cmd.Segments {
			if len(cmd.Segments) > 1 && j != 0 && !lastSegHasNoCmd {
				fmt.Fprint(w, cmdPathSep)
			}
			fmt.Fprint(w, seg.Matched.Name)

			if seg.Matched.Cmd != nil {
				path = append(path, seg.Matched.Cmd.Name())
			} else {
				path = append(path, seg.Matched.Name)
			}
			lastSegHasNoCmd = (seg.Matched.Cmd == nil)
			cmdHasEnv = cmdHasEnv || saveFlowEnv(w, seg.Env, path, envPathSep,
				bracketLeft, bracketRight, envKeyValSep,
				!cmdHasEnv && j == len(cmd.Segments)-1)
		}
	}
}

func saveFlowEnv(
	w io.Writer,
	env core.ParsedEnv,
	prefixPath []string,
	pathSep string,
	bracketLeft string,
	bracketRight string,
	envKeyValSep string,
	useArgsFmt bool) bool {

	if len(env) == 0 {
		return false
	}

	isAllArgs := true
	for _, v := range env {
		if !v.IsArg {
			isAllArgs = false
			break
		}
	}

	prefix := strings.Join(prefixPath, pathSep) + pathSep

	var kvs []string
	for k, v := range env {
		if strings.HasPrefix(k, prefix) && len(k) != len(prefix) {
			k = strings.Join(v.MatchedPath[len(prefixPath):], pathSep)
		}
		kvs = append(kvs, fmt.Sprintf("%v%s%v", k, envKeyValSep, quoteIfHasSpace(v.Val)))
	}

	format := bracketLeft + "%s" + bracketRight
	if isAllArgs && useArgsFmt {
		format = " %s"
	}
	fmt.Fprintf(w, format, strings.Join(kvs, " "))
	return true
}

func getFlowCmdPath(
	flow *core.ParsedCmds,
	currCmdIdx int,
	getArgFromFlow bool,
	argv core.ArgVals,
	cc *core.Cli,
	env *core.Env,
	expectExists bool,
	argName string) (cmdPath string, filePath string) {

	var arg string
	if !getArgFromFlow {
		arg = argv.GetRaw(argName)
		if len(arg) == 0 {
			panic(core.NewCmdError(flow.Cmds[currCmdIdx], "arg '"+arg+"' is empty"))
		}
	} else {
		arg = tailModeCallArg(flow, currCmdIdx, argv, argName)
	}
	cmdPath = normalizeCmdPath(arg,
		cc.Cmds.Strs.PathSep, cc.Cmds.Strs.PathAlterSeps)
	if len(cmdPath) == 0 {
		origin := argv.GetRaw(argName)
		if len(origin) == 0 {
			panic(core.NewCmdError(flow.Cmds[currCmdIdx],
				fmt.Sprintf("arg '%s' is empty", argName)))
		} else {
			panic(core.NewCmdError(flow.Cmds[currCmdIdx],
				fmt.Sprintf("arg '%s' is empty after normalizing: %s -> %s",
					argName, origin, cmdPath)))
		}
	}

	flowExt := env.GetRaw("strs.flow-ext")
	root := getFlowRoot(env, flow.Cmds[currCmdIdx])

	filePath = filepath.Join(root, cmdPath) + flowExt
	if !expectExists && fileExists(filePath) {
		if !env.GetBool("sys.interact") {
			panic(core.NewCmdError(flow.Cmds[currCmdIdx],
				fmt.Sprintf("flow '%s' file '%s' exists", cmdPath, filePath)))
		} else {
			return
		}
	}
	if expectExists && !fileExists(filePath) {
		panic(core.NewCmdError(flow.Cmds[currCmdIdx],
			fmt.Sprintf("flow '%s' file '%s' not exists", cmdPath, filePath)))
	}
	return
}

func checkAndConfirmIfFlowHasParseError(screen core.Screen, flow *core.ParsedCmds, env *core.Env) bool {
	for _, cmd := range flow.Cmds {
		if cmd.ParseResult.Error == nil {
			continue
		}
		screen.Print(display.ColorTip("[confirm]", env) + " flow has parse error, " +
			"type " + display.ColorWarn("'y'", env) + " and press enter to force save:\n")
		utils.UserConfirm()
		break
	}
	return true
}

func getFlowRoot(env *core.Env, cmd core.ParsedCmd) string {
	root := env.GetRaw("sys.paths.flows")
	if len(root) == 0 {
		panic(core.NewCmdError(cmd, "env 'sys.paths.flows' is empty"))
	}
	return root
}
