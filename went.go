package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

import "github.com/chzyer/readline"
import "github.com/mgutz/ansi"

type irc_T struct {
	src       string
	dest      string
	cmd       string
	body      string
	is_action bool
}

type config_T struct {
	self      func(string) string
	users     func(string) string
	chans     func(string) string
	errors    func(string) string
	promptFmt string
	msgFmt    string
	destFmt   string
	actionFmt string
	errFmt	string
	verbose   bool
	out	io.Writer
}

var Nick string
var Window string

// Color functions/setup
func randColor(str string) string {
	var hash uint64
	for _, c := range str {
		hash += uint64(c)
	}
	return ansi.Color(str, strconv.FormatUint(hash%256, 10)+"")
}
func autoColor(col string, auto bool) func(string) string {
	if auto && len(col) == 0 {
		return randColor
	}
	return ansi.ColorFunc(col)
}
func MakeConfig(
	config [9]string,
	auto bool,
	verbose bool,
	stdout io.Writer,
) (strut config_T) {

	strut = config_T{
		self:      autoColor(config[0], auto),
		users:     autoColor(config[1], auto),
		chans:     autoColor(config[2], auto),
		errors:    autoColor(config[3], auto),
		promptFmt: config[4],
		msgFmt:    config[5],
		destFmt:   config[6],
		actionFmt: config[7],
		errFmt:	config[8],
		out:		stdout,
		verbose:   verbose,
	}
	return
}

// Global variable setters
func setWin(win string, conf config_T, rl *readline.Instance) {
	Window = win
	rl.SetPrompt(fmt.Sprintf(conf.promptFmt, conf.self(Nick), conf.chans(win)))
	dispErr(conf, "WENT", "Window focus changed to", conf.chans(win)+"\n")
}
func setNick(newNick string, conf config_T, rl *readline.Instance) {
	if Window == Nick {
		Nick = newNick
		setWin(Nick, conf, rl)
	} else {
		Nick = newNick
		setWin(Window, conf, rl)
	}
}

// IRC Message Senders
func sendPM(msg []string) (out string, err error) {
	if len(msg) < 3 {
		err = errors.New("Usage: /msg <channel/user> <message>")
		return
	}
	out = fmt.Sprintf("PRIVMSG %s :%s", msg[1], msg[2])
	return
}
func sendCmd(msg []string, cmd string, usage string, n int) (out string, err error) {
	if len(msg) < n {
		err = errors.New("Usage: " + usage)
		return
	}
	msg[0] = cmd
	out = strings.Join(msg, " ")
	return
}
func sendToChan(msg []string, cmd string, usage string) (out string, err error) {
	if len(msg) < 2 && Window[0] == '#' {
		msg = append(msg, Window)
	}
	return sendCmd(msg, cmd, usage, 2)
}

// Format messages to print on screen
func dispMsg(strut irc_T, conf config_T) {
	target := ""
	if conf.verbose || strut.dest != Window || (strut.dest == Nick && strut.src != Window) {
		target = fmt.Sprintf(conf.destFmt, conf.chans(strut.dest))
	}
	if strut.is_action {
		fmt.Fprintf(conf.out, conf.actionFmt,
			target, strut.src, strut.body)
	} else {
		fmt.Fprintf(conf.out, conf.msgFmt,
			target, strut.src, strut.body)
	}
}
func dispErr(conf config_T, code string, target string, body string) {
	fmt.Fprintf(conf.out, conf.errFmt, conf.errors(code), target, body)
}

func procInput(serv net.Conn, conf config_T, rl *readline.Instance) {
	for {
		line, err := rl.Readline()
		if err != nil {
			if err.Error() != "Interrupt" {
				fmt.Println("Exiting:", err)
			}
			rl.Close()
			fmt.Fprintf(serv, "QUIT Leaving...\n")
			return
		}
		msg := strings.SplitN(line, " ", 3)
		out := ""
		if len(line) > 1 && line[0] == '/' {
			switch msg[0] {
			case "/m", "/msg", "/send", "/s":
				out, err = sendPM(msg)
				if err == nil {
					setWin(msg[1], conf, rl)
					strut := irc_T{conf.self(Nick), msg[1], "", msg[2] + "\n", false}
					dispMsg(strut, conf)
				}
			case "/me", "/action":
				if len(msg) < 2 {
					err = errors.New("/me <message>")
				} else {
					strut := irc_T{conf.self(Nick), Window, "", msg[1] + "\n", true}
					dispMsg(strut, conf)
					out = fmt.Sprintf("PRIVMSG %s :\001ACTION %s \001", Window, msg[1])
				}
				//msg = []string{"PRIVMSG", Window, ":\001ACTION " + msg[1] + "\001"}
				//out, err = sendPM(msg)
			case "/who":
				out, err = sendToChan(msg, "WHO", "/who <channel>")
			case "/whois":
				out, err = sendCmd(msg, "WHOIS", "/whois <user/channel/op>", 2)
			case "/whowas":
				out, err = sendCmd(msg, "WHOIS", "/whowas <user/channel/op>", 2)
			case "/j", "/join":
				out, err = sendCmd(msg, "JOIN", "/msg <channel>", 2)
				if err == nil {
					setWin(msg[1], conf, rl)
				}
			case "/p", "/part":
				out, err = sendToChan(msg, "PART", "/part [<channels>]")
				if err == nil {
					setWin(Nick, conf, rl)
				}
			case "/topic":
				out, err = sendToChan(msg, "TOPIC", "/topic [<channel>] [<new toipic>]")
			case "/names":
				out, err = sendToChan(msg, "NAMES", "/names [<channel>]")
			case "/n", "/nick":
				out, err = sendCmd(msg, "NICK", "/nick <newnick>", 2)
				if err == nil {
					setNick(msg[1], conf, rl)
				}
			case "/w", "/cur", "/win", "/window":
				if len(msg) < 2 {
					err = errors.New("/window <channel/user>")
				} else {
					setWin(msg[1], conf, rl)
				}
			case "/q":
				msg[0] = "QUIT"
				if len(msg) == 1 {
					msg = append(msg, "Leaving...")
				}
				out = strings.Join(msg, " ")
				rl.Close()
			default:
				out = line[1:]
			}
		} else { // Line does not start with /
			if line != "" {
				if Window != Nick {
					msg = []string{"PRIVMSG", Window, line}
					out, err = sendPM(msg)
					if err == nil {
						strut := irc_T{conf.self(Nick), Window, "", line + "\n", false}
						dispMsg(strut, conf)
					}
				} else {
					err = errors.New("Error: Use /w to set current window")
				}
			}
		}
		if err != nil {
			fmt.Fprintln(rl.Stdout(), err)
		} else {
			fmt.Fprintln(serv, out)
		}
	}
}

// Overly simplistic irc parsing functions
func findSrc(msg string, nickColor func(string) string) (string, string) {
	src := "-!- "
	for i, c := range msg {
		if c == ' ' {
			msg = msg[i:]
			break
		} else if c == '!' {
			src = msg[:i]
		}
	}
	return nickColor(src), msg
}
func makeStrut(msg string, nickColor func(string) string) (strut irc_T) {
	if msg[0] == ':' { // Message has a source
		strut.src, msg = findSrc(msg[1:], nickColor)
	}
	for i, c := range msg {
		msg = strings.Trim(msg, " ")
		if c == ':' {
			strut.cmd = strings.Trim(msg[:i], " :")
			strut.body = strings.Trim(msg[i:], " :")
			if strut.body[0] == '\001' && strut.body[len(strut.body)-3] == '\001' {
				split := strings.SplitN(strut.body, " ", 2)
				if split[0] == "\001ACTION" {
					strut.is_action = true
					strut.body = split[1]
				}
			}
			break
		}
		t := strings.SplitN(msg, " ", 2)
		strut.cmd = strings.Trim(t[0], " :")
		strut.body = strings.Trim(t[1], " :")
	}
	return
}

func procServer(serv net.Conn, conf config_T) {
	servReader := *bufio.NewReader(serv)
	fmt.Fprintln(serv, "NICK ", Nick)
	fmt.Fprintf(serv, "USER %s 8 * : %s \n", Nick, Nick)
	for {
		msg, err := servReader.ReadString('\n')
		if err != nil {
			if err.Error() != "EOF" {
				fmt.Println("Exiting:", err)
			}
			return
		}
		strut := makeStrut(msg, conf.users)
		cmdSlice := strings.SplitN(strut.cmd, " ", 3)
		switch cmdSlice[0] {
		case "PING":
			fmt.Fprint(serv, "PONG :", strut.body)
		case "PRIVMSG", "NOTICE":
			strut.dest = cmdSlice[1]
			dispMsg(strut, conf)
		case "JOIN":
			dispErr(conf, cmdSlice[0], strut.src, "has joined "+conf.chans(strut.body))
		case "PART":
			dispErr(conf, cmdSlice[0], strut.src, "has left "+conf.chans(strut.body))
		case "NICK":
			dispErr(conf, cmdSlice[0], strut.src, "is now known as "+conf.users(strut.body))
		case "MODE", "324":
			dispErr(conf, cmdSlice[0], cmdSlice[1], strut.body)
		case "ERROR":
			dispErr(conf, cmdSlice[0], strut.src, strut.body)
		default:
			reply, err := strconv.Atoi(cmdSlice[0])
			switch {
			case (reply > 250 && reply < 267) || (reply > 0 && reply < 5) || (reply > 370 && reply < 377):
				t := strings.SplitN(msg, " ", 4)
				strut.body = t[len(t)-1]
				dispErr(conf, "INFO", "", strut.body)
			case reply > 399:
				dispErr(conf, "ERROR", strut.src, strut.body)
			case (reply > 364 && reply < 369) || reply == 353:
				dispErr(conf, "NAMES", "", strut.body)
			case (reply > 301 && reply < 320) || (reply > 351 && reply < 356) || reply == 330 || reply == 360:
				dispErr(conf, "WHO", cmdSlice[2], strut.body)
			case (reply > 330 && reply < 334):
				t := strings.SplitN(msg, " ", 4)
				strut.body = t[len(t)-1]
				dispErr(conf, "TOPIC", "", strut.body)
			case (reply > 199 && reply < 220) || reply == 5:
				// Ignore, stats and trace messages
			case err != nil:
				fmt.Fprintf(conf.out, " Unknown message type: %s", msg)
				fmt.Fprintf(conf.out, " ??? %s | %s | %s | %s", err, strut.src, strut.cmd, strut.body)
			default:
				dispErr(conf, strut.src, strut.cmd, strut.body)
			}
		}
	}
}

func main() {
	var config [9]string
	host, port, history, auto, verbose := "", "", "", false, false
	flag.StringVar(&host, "s", "irc.foonetic.net",
		"Hostname of the irc server.")
	flag.StringVar(&Nick, "n", "lhk-go",
		"Your nick/user/full name.")
	flag.StringVar(&port, "p", "6667",
		"Port of the irc server.")
	flag.StringVar(&history, "histfile", ".went_history",
		"Path to persistent history file.")
	flag.StringVar(&config[0], "self-color", "cyan+b",
		"Color of own nick.")
	flag.StringVar(&config[1], "user-color", "",
		"Color of others' nicks.")
	flag.StringVar(&config[2], "chan-color", "",
		"Color of channel strings.")
	flag.StringVar(&config[3], "error-color", "red+b",
		"Color of error strings.")
	flag.StringVar(&config[4], "promptfmt", "[%s.%s] ",
		"Prompt format string in fmt.printf format with 2 args.")
	flag.StringVar(&config[5], "msgfmt", "%s< %s> %s",
		"Message format string in fmt.printf format with 3 args.")
	flag.StringVar(&config[6], "destfmt", "[%s] ",
		"Destination format string in fmt.printf format with 1 arg.")
	flag.StringVar(&config[7], "actionfmt", "%s* %s ~> %s",
		"Action format string in fmt.printf format with 3 args.")
	flag.StringVar(&config[8], "errorfmt", "-%s- %s %s",
		"Error format string in fmt.printf format with 3 args.")
	flag.BoolVar(&auto, "auto-color", true,
		"Enable randomly colored strings.")
	flag.BoolVar(&verbose, "verbose", true,
		"Always show message destinations.")
	flag.Parse()

	serv, err := net.Dial("tcp", host+":"+port)
	if err != nil {
		fmt.Println("Exiting:", err)
		return
	}
	defer serv.Close()

	rl, err := readline.NewEx(
		&readline.Config{
			UniqueEditLine:  true,
			InterruptPrompt: "^C",
			HistoryFile:     history,
			//Prompt:          fmt.Sprintf(conf.promptFmt, conf.self(Nick), conf.chans(Nick)),
		})
	if err != nil {
		panic(err)
	}
	defer rl.Close()

	conf := MakeConfig(config, auto, verbose, rl.Stdout())
	Window = Nick
	rl.SetPrompt(fmt.Sprintf(conf.promptFmt, conf.self(Nick), conf.chans(Nick)))

	go procInput(serv, conf, rl)
	procServer(serv, conf)
}
