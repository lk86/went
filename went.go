
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	//	"os"
	"errors"
	"strconv"
	"strings"
)

import "github.com/chzyer/readline"
import "github.com/mgutz/ansi"

type irc struct {
	src  string
	cmd  string
	body string
	is_action bool
}

type color_T struct {
	self func(string) string
	nicks func(string) string
	chans func(string) string
	errors func(string) string
}

var window string
var nick string
var colors color_T


// Color functions/setup
func rand_colors(str string) string {
	var hash uint64
	for _, c := range str {
		hash += uint64(c)
	}
	return ansi.Color(str, strconv.FormatUint(hash % 256, 10) + "")
}
func auto_colors(col string, auto bool) func(string) string {
	if auto && len(col) == 0 {
		return rand_colors
	}
	return ansi.ColorFunc(col)
}
func make_colors(self string, nicks string, chans string, errors string, auto bool) (strut color_T) {
	strut = color_T {
		self: auto_colors(self, auto),
		nicks: auto_colors(nicks, auto),
		chans: auto_colors(chans, auto),
		errors: auto_colors(errors, auto),
	}
	return
}

// Display Helpers
func set_window(win string, colors color_T, rl *readline.Instance) {
	window = win
	rl.SetPrompt(fmt.Sprintf("[%s.%s] ", colors.self(nick), colors.chans(win)))
	fmt.Fprintf(rl.Stdout(), "-WENT- Window focus changed to %s\n", colors.chans(win))
}
func show_msg(dest string, body string, colors color_T) string {
	if dest == window {
		return fmt.Sprintf("< %s> %s", colors.self(nick), body)
	}
	return fmt.Sprintf("[%s] < %s> %s", colors.chans(dest), colors.self(nick), body)
}

// IRC Message Functions
func privmsg(msg []string) (out string, err error) {
	if len(msg) < 3 {
		err = errors.New("Usage: /msg <channel/user> <message>")
		return
	}
	out = fmt.Sprintf("PRIVMSG %s :%s", msg[1], msg[2])
	return
}
func std_cmd(msg []string, cmd string, usage string, n int) (out string, err error) {
	if len(msg) < n {
		err = errors.New("Usage: " + usage)
		return
	}
	msg[0] = cmd
	out = strings.Join(msg, " ")
	return
}
func chan_cmd(msg []string, cmd string, usage string) (out string, err error) {
	if len(msg) < 2 && window[0] == '#' {
		msg = append(msg, window)
	}
	return std_cmd(msg, cmd, usage, 2)
}

func proc_input(conn net.Conn, colors color_T, rl *readline.Instance) {
	for {
		line, err := rl.Readline()
		if err != nil {
			if err.Error() != "Interrupt" {
				fmt.Println("Exiting:", err)
			}
			rl.Close()
			fmt.Fprintf(conn, "QUIT Leaving...\n")
			return
		}
		msg := strings.SplitN(line, " ", 3)
		out := ""
		if len(line) > 1 && line[0] == '/' {
			switch msg[0] {
			case "/m", "/msg", "/send", "/s":
				out, err = privmsg(msg)
				if err == nil {
					set_window(msg[1], colors, rl)
					fmt.Fprintln(rl.Stdout(), show_msg(msg[1], msg[2], colors))
				}
			case "/me", "/action":
				if len(msg) < 2 {
					err = errors.New("/me <message>")
				}
				msg = []string{"PRIVMSG", window, "\001ACTION " + msg[1] + "\001"}
				out, err = privmsg(msg)
			case "/who":
				out, err = chan_cmd(msg, "WHO", "/who <channel>")
			case "/whois":
				out, err = std_cmd(msg, "WHOIS", "/whois <user/channel/op>", 2)
			case "/whowas":
				out, err = std_cmd(msg, "WHOIS", "/whowas <user/channel/op>", 2)
			case "/j", "/join":
				out, err = std_cmd(msg, "JOIN", "/msg <channel>", 2)
				if err == nil {
					set_window(msg[1], colors, rl)
				}
			case "/p", "/part":
				out, err = chan_cmd(msg, "PART", "/part [<channels>]")
				if err == nil {
					set_window(nick, colors, rl)
				}
			case "/topic":
				out, err = chan_cmd(msg, "TOPIC", "/topic [<channel>] [<new toipic>]")
			case "/names":
				out, err = chan_cmd(msg, "NAMES", "/names [<channel>]")
			case "/n", "/nick":
				out, err = std_cmd(msg, "NICK", "/nick <newnick>", 2)
				if window == nick && err == nil {
					set_window(msg[1], colors, rl)
					nick = msg[1]
				}
			case "/w", "/cur", "/win", "/window":
				if len(msg) < 2 {
					err = errors.New("/window <channel/user>")
				} else {
					set_window(msg[1], colors, rl)
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
				if window != nick {
					msg = []string{"PRIVMSG", window, line}
					out, err = privmsg(msg)
					fmt.Fprintln(rl.Stdout(), show_msg(msg[1], msg[2], colors))
				} else {
					err = errors.New("Error: Use /w to set current window")
				}
			}
		}
		if err != nil {
			fmt.Fprintln(rl.Stdout(), err)
		} else {
			fmt.Fprintln(conn, out)
		}
	}
}

func get_src(msg string, nick_color func(string) string) (string, string) {
	src := "-!- "
	for i, c := range msg {
		if c == ' ' {
			msg = msg[i:]
			break
		} else if c == '!' {
			src = msg[:i]
		}
	}
	return nick_color(src), msg
}

func parse_msg(msg string, nick_color func(string) string) (strut irc) {
	if msg[0] == ':' { // Message has a source
		strut.src, msg = get_src(msg[1:], nick_color)
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

func at_window(target string) bool {
	return window[0] != '#' && target == nick || window == target
}

func proc_conn(conn net.Conn, colors color_T, out io.Writer) {
	tcpread := *bufio.NewReader(conn)
	fmt.Fprintln(conn, "NICK ", nick)
	fmt.Fprintf(conn, "USER %s 8 * : %s \n", nick, nick)
	for {
		msg, err := tcpread.ReadString('\n')
		if err != nil {
			if err.Error() != "EOF" {
				fmt.Println("Exiting:", err)
			}
			return
		}
		strut := parse_msg(msg, colors.nicks)
		splcmd := strings.SplitN(strut.cmd, " ", 3)
		switch strings.Trim(splcmd[0], " :") {
		case "PING":
			fmt.Fprint(conn, "PONG :", strut.body)
		case "PRIVMSG", "NOTICE":
			target := ""
			if !at_window(splcmd[1]) {
				target = "[" + colors.chans(splcmd[1]) + "] "
			}
			if strut.is_action {
				fmt.Fprintf(out, "%s* %s ~> %s",
					target, strut.src, strut.body)
			} else {
				fmt.Fprintf(out, "%s< %s> %s",
					target, strut.src, strut.body)
			}
		case "ERROR":
			fmt.Fprintf(out, "-%s- %s %s",
				colors.errors("ERROR"), strut.src, strut.body)
		case "JOIN":
			fmt.Fprintf(out, "-%s- %s has joined %s",
				"JOIN", strut.src, strut.body)
		case "PART":
			fmt.Fprintf(out, "-%s- %s has left %s",
				"PART", strut.src, strut.body)
		case "MODE", "324":
			fmt.Fprintf(out, "-%s- %s %s",
				"MODE", splcmd[1], strut.body)
		default:
			reply, err := strconv.Atoi(splcmd[0])
			switch {
			case (reply > 250 && reply < 267) || (reply > 0 && reply < 5) || (reply > 370 && reply < 377):
				t := strings.SplitN(msg, " ", 4)
				strut.body = t[len(t) - 1]
				fmt.Fprintf(out, "-%s- %s", "INFO", strut.body)
			case reply > 399:
				fmt.Fprintf(out, "-%s- %s %s", colors.errors("ERROR"), strut.src, strut.body)
			case (reply > 364 && reply < 369) || reply == 353:
				fmt.Fprintf(out, "-%s- %s", "NAMES", strut.body)
			case (reply > 301 && reply < 320) || (reply > 351 && reply < 356) ||reply == 330 || reply == 360:
				fmt.Fprintf(out, "-%s- %s: %s", "WHO", splcmd[2], strut.body)
			case (reply > 330 && reply < 334):
				t := strings.SplitN(msg, " ", 4)
				strut.body = t[len(t) - 1]
				fmt.Fprintf(out, "-%s- %s", "TOPIC", strut.body)
			case (reply > 199 && reply < 220) || reply == 5:
				// Ignore, stats and trace messages
			case err != nil:
				fmt.Fprintf(out, " Unknown message type: %s", msg)
				fmt.Fprintf(out, " ??? %s | %s | %s | %s", err, strut.src, strut.cmd, strut.body)
			default:
				fmt.Fprintf(out, "%s %s %s", strut.src, strut.cmd, strut.body)
			}
		}
	}
}

func main() {
	host, port, self, nicks, chans, errors, hist, auto :=
					"", "", "", "", "", "", "", false
	flag.StringVar(&host, "s", "irc.foonetic.net",
		"Hostname of the irc server.")
	flag.StringVar(&nick, "n", "lhk-go",
		"Your nick/user/full name.")
	flag.StringVar(&port, "p", "6667",
		"Port of the irc server.")
	flag.StringVar(&hist, "histfile", ".went_history",
		"Path to persistent history file.")
	flag.StringVar(&self, "self-color", "cyan+b",
		"Color of own nick.")
	flag.StringVar(&nicks, "nick-color", "",
		"Color of others' nicks.")
	flag.StringVar(&chans, "chan-color", "red+b",
		"Color of channel strings.")
	flag.StringVar(&errors, "error-color", "red",
		"Color of error strings.")
	flag.BoolVar(&auto, "auto-color", true,
		"Enable randomly colored strings.")
	flag.Parse()

	colors := make_colors(self, nicks, chans, errors, auto)

	conn, err := net.Dial("tcp", host+":"+port)
	if err != nil {
		fmt.Println("Exiting:", err)
		return
	}
	defer conn.Close()

	rl, err := readline.NewEx(
		&readline.Config{
			UniqueEditLine:  true,
			InterruptPrompt: "^C",
			HistoryFile: hist,
			Prompt:          fmt.Sprintf("[%s.%s] ", colors.self(nick), colors.chans(nick)),
		})
	if err != nil {
		panic(err)
	}
	defer rl.Close()

	window = nick
	go proc_input(conn, colors, rl)
	proc_conn(conn, colors, rl.Stdout())
}
