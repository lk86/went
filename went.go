package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"errors"
	"strconv"
	"strings"
)

import "github.com/chzyer/readline"
import "github.com/mgutz/ansi"

type irc_T struct {
	src       string
	cmd       string
	body      string
	is_action bool
}

type config_T struct {
	prompt string
	self   func(string) string
	users  func(string) string
	chans  func(string) string
	errors func(string) string
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
		prompt string,
		self string,
		users string,
		chans string,
		errors string,
		auto bool,
		) (strut config_T) {

	strut = config_T{
		self:   autoColor(self, auto),
		users:  autoColor(users, auto),
		chans:  autoColor(chans, auto),
		errors: autoColor(errors, auto),
		prompt: prompt,
	}
	return
}

// Display Helpers
func setWin(win string, conf config_T, rl *readline.Instance) {
	Window = win
	rl.SetPrompt(fmt.Sprintf(conf.prompt, conf.self(Nick), conf.chans(win)))
	fmt.Fprintf(rl.Stdout(), "-WENT- Window focus changed to %s\n", conf.chans(win))
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
func dispMsg(dest string, body string, conf config_T) string {
	if dest == Window {
		return fmt.Sprintf("< %s> %s", conf.self(Nick), body)
	}
	return fmt.Sprintf("[%s] < %s> %s", conf.chans(dest), conf.self(Nick), body)
}

// IRC Message Functions
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
					fmt.Fprintln(rl.Stdout(), dispMsg(msg[1], msg[2], conf))
				}
			case "/me", "/action":
				if len(msg) < 2 {
					err = errors.New("/me <message>")
				}
				msg = []string{"PRIVMSG", Window, "\001ACTION " + msg[1] + "\001"}
				out, err = sendPM(msg)
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
					fmt.Fprintln(rl.Stdout(), dispMsg(msg[1], msg[2], conf))
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

func isWindow(conf config_T, target string) bool {
	return Window[0] != '#' && target == Nick || Window == target
}

func procServer(serv net.Conn, conf config_T, stdout io.Writer) {
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
			target := ""
			if !isWindow(conf, cmdSlice[1]) {
				target = "[" + conf.chans(cmdSlice[1]) + "] "
			}
			if strut.is_action {
				fmt.Fprintf(stdout, "%s* %s ~> %s",
					target, strut.src, strut.body)
			} else {
				fmt.Fprintf(stdout, "%s< %s> %s",
					target, strut.src, strut.body)
			}
		case "ERROR":
			fmt.Fprintf(stdout, "-%s- %s %s",
				conf.errors("ERROR"), strut.src, strut.body)
		case "JOIN":
			fmt.Fprintf(stdout, "-%s- %s has joined %s",
				"JOIN", strut.src, conf.chans(strut.body))
		case "PART":
			fmt.Fprintf(stdout, "-%s- %s has left %s",
				"PART", strut.src, conf.chans(strut.body))
		case "NICK":
			fmt.Fprintf(stdout, "-%s- %s is now known as %s",
				"NICK", strut.src, conf.users(strut.body))
		case "MODE", "324":
			fmt.Fprintf(stdout, "-%s- %s %s",
				"MODE", cmdSlice[1], strut.body)
		default:
			reply, err := strconv.Atoi(cmdSlice[0])
			switch {
			case (reply > 250 && reply < 267) || (reply > 0 && reply < 5) || (reply > 370 && reply < 377):
				t := strings.SplitN(msg, " ", 4)
				strut.body = t[len(t)-1]
				fmt.Fprintf(stdout, "-%s- %s", "INFO", strut.body)
			case reply > 399:
				fmt.Fprintf(stdout, "-%s- %s %s", conf.errors("ERROR"), strut.src, strut.body)
			case (reply > 364 && reply < 369) || reply == 353:
				fmt.Fprintf(stdout, "-%s- %s", "NAMES", strut.body)
			case (reply > 301 && reply < 320) || (reply > 351 && reply < 356) || reply == 330 || reply == 360:
				fmt.Fprintf(stdout, "-%s- %s: %s", "WHO", cmdSlice[2], strut.body)
			case (reply > 330 && reply < 334):
				t := strings.SplitN(msg, " ", 4)
				strut.body = t[len(t)-1]
				fmt.Fprintf(stdout, "-%s- %s", "TOPIC", strut.body)
			case (reply > 199 && reply < 220) || reply == 5:
				// Ignore, stats and trace messages
			case err != nil:
				fmt.Fprintf(stdout, " Unknown message type: %s", msg)
				fmt.Fprintf(stdout, " ??? %s | %s | %s | %s", err, strut.src, strut.cmd, strut.body)
			default:
				fmt.Fprintf(stdout, "%s %s %s", strut.src, strut.cmd, strut.body)
			}
		}
	}
}

func main() {
	host, port, prompt, hist, self, users, chans, errors, auto :=
		"","","","","","","","", false
	flag.StringVar(&host, "s", "irc.foonetic.net",
		"Hostname of the irc server.")
	flag.StringVar(&Nick, "n", "lhk-go",
		"Your nick/user/full name.")
	flag.StringVar(&port, "p", "6667",
		"Port of the irc server.")
	flag.StringVar(&prompt, "prompt", "[%s.%s] ",
		"Prompt string in fmt.printf format with 2 args.")
	flag.StringVar(&hist, "histfile", ".went_history",
		"Path to persistent history file.")
	flag.StringVar(&self, "self-color", "cyan+b",
		"Color of own nick.")
	flag.StringVar(&users, "nick-color", "",
		"Color of others' nicks.")
	flag.StringVar(&chans, "chan-color", "red+b",
		"Color of channel strings.")
	flag.StringVar(&errors, "error-color", "red",
		"Color of error strings.")
	flag.BoolVar(&auto, "auto-color", true,
		"Enable randomly colored strings.")
	flag.Parse()

	conf := MakeConfig(prompt, self, users, chans, errors, auto)
	Window = Nick

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
			HistoryFile:     hist,
			Prompt:          fmt.Sprintf(conf.prompt, conf.self(Nick), conf.chans(Nick)),
		})
	if err != nil {
		panic(err)
	}
	defer rl.Close()

	go procInput(serv, conf, rl)
	procServer(serv, conf, rl.Stdout())
}
