package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	//	"os"
	"errors"
	"strings"
)

import "gopkg.in/readline.v1"

type irc struct {
	src  string
	cmd  string
	body string
}

var window string
var nick string

func set_window(win string, rl *readline.Instance) {
	window = win
	rl.SetPrompt("[" + window + "] ")
}

func privmsg(msg []string) (out string, err error) {
	if len(msg) < 3 {
		err = errors.New("Usage: /msg <channel/user> <message>")
		return
	}
	out = fmt.Sprintf("PRIVMSG %s :%s\n", msg[1], msg[2])
	return
}

func join(msg []string) (out string, err error) {
	if len(msg) < 2 {
		err = errors.New("Usage: /join <channels>")
		return
	}
	out = fmt.Sprintln(strings.Join(msg, " "))
	return
}

func set_nick(msg []string, rl *readline.Instance) (out string, err error) {
	if len(msg) != 2 {
		err = errors.New("Usage: /nick <new_nickname>")
		return
	}
	out = strings.Join(msg, " ")
	if window == nick {
		set_window(msg[1], rl)
	}
	nick = msg[1]
	return
}

func chan_cmd(msg []string, usage string) (out string, err error) {
	if len(msg) < 2 && window[0] == '#' {
		msg = append(msg, window)
	}
	if len(msg) < 2 {
		err = errors.New(usage)
		return
	}
	out = strings.Join(msg, " ")
	return
}

func proc_input(conn net.Conn, rl *readline.Instance) {
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
				set_window(msg[1], rl)
			case "/j", "/join":
				msg[0] = "JOIN"
				out, err = join(msg)
				set_window(msg[1], rl)
			case "/p", "/part":
				msg[0] = "PART"
				out, err = chan_cmd(msg, "Usage: /part [<channels>]")
				set_window(nick, rl)
			case "/t", "/topic":
				msg[0] = "TOPIC"
				out, err = chan_cmd(msg,
					"Usage: /topic [<channel>] [<new toipic>]")
			case "/ns", "/names":
				msg[0] = "NAMES"
				out, err = chan_cmd(msg, "Usage: /names [<channel>]")
			case "/n", "/nick":
				msg[0] = "NICK"
				set_nick(msg, rl)
			case "/w", "/cur", "/win", "/window":
				if len(msg) < 2 {
					err = errors.New("/window <channel/user>")
				} else {
					set_window(msg[1], rl)
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
		} else {
			if line != "" {
				if window != nick {
					msg = []string{"PRIVMSG", window, line}
					out, err = privmsg(msg)
				} else {
					err = errors.New("Error: Use /w to set current window")
				}
			}
		}
		if err != nil {
			fmt.Fprintln(rl.Stdout(), err)
		} else {
			fmt.Fprint(conn, out)
		}
	}
}

func get_src(msg string) (string, string) {
	src := "-!- "
	for i, c := range msg {
		if c == ' ' {
			msg = msg[i:]
			break
		} else if c == '!' {
			src = msg[:i]
		}
	}
	return src, msg
}

func parse_msg(msg string) (strut irc) {
	if msg[0] == ':' { // Message has a source
		strut.src, msg = get_src(msg[1:])
	}
	for i, c := range msg {
		if c == ':' {
			strut.cmd = strings.Trim(msg[:i], " :")
			strut.body = strings.Trim(msg[i:], " :")
			break
		}
	}
	return
}

func proc_conn(conn net.Conn, out io.Writer) {
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
		strut := parse_msg(msg)
		splcmd := strings.SplitN(strut.cmd, " ", 2)
		switch splcmd[0] {
		case "PING":
			fmt.Fprint(conn, "PONG :", strut.body)
		case "PRIVMSG", "NOTICE":
			if window[0] != '#' && splcmd[1] == nick || window == splcmd[1] {
				//if window == splcmd[1] {
				fmt.Fprintf(out, "< %s> %s",
					strut.src, strut.body)
			} else {
				fmt.Fprintf(out, "@%s: < %s> %s",
					splcmd[1], strut.src, strut.body)
			}
		case "JOIN":
			fmt.Fprintf(out, "-!- %s has joined %s",
				strut.src, strut.body)
		case "MODE":
			fmt.Fprintf(out, "-MODES- %s %s",
				splcmd[1], strut.body)
		case "001", "002", "003", "004",
			"251", "255", "265", "266",
			"332", "353":
			fmt.Fprintf(out, "%s %s", strut.src, strut.body)
		case "005", "252", "254", "309", "366", "375", "376":
			// Ignore these, not useful/too hard to parse
		default:
			if len(strut.body) > 0 && strut.body[0] == '-' {
				fmt.Fprintf(out, "%s %s", strut.src, strut.body)
			} else {
				fmt.Fprintf(out, "%s %s %s", strut.src, strut.cmd, strut.body)
			}
		}
	}
}

func main() {
	host, port := "", ""
	flag.StringVar(&host, "s", "irc.foonetic.net",
		"Hostname of the irc server.")
	flag.StringVar(&nick, "n", "lhk-go",
		"Your nick/user/full name.")
	flag.StringVar(&port, "p", "6667",
		"Port of the irc server.")
	flag.Parse()

	conn, err := net.Dial("tcp", host+":"+port)
	if err != nil {
		fmt.Println("Exiting:", err)
		return
	}
	defer conn.Close()

	rl, err := readline.NewEx(
		&readline.Config{
			UniqueEditLine:  false,
			InterruptPrompt: "^C",
			Prompt:          "[" + nick + "] ",
		})
	if err != nil {
		panic(err)
	}
	defer rl.Close()

	window = nick
	go proc_input(conn, rl)
	proc_conn(conn, rl.Stdout())
}
