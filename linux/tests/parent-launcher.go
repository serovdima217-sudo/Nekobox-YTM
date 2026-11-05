// SPDX-License-Identifier: GPL-3.0-or-later
// Тестовый лаунчер: дочерний процесс имеет другой исполняемый файл.
package main
import("os";"os/exec")
func main(){args:=os.Args[1:];if len(args)>0&&args[0]=="--unlink-self"{p,e:=os.Executable();if e!=nil{panic(e)};if e=os.Remove(p);e!=nil{panic(e)};args=args[1:]};if len(args)==0{os.Exit(2)};c:=exec.Command(args[0],args[1:]...);c.Stdin=os.Stdin;c.Stdout=os.Stdout;c.Stderr=os.Stderr;if e:=c.Run();e!=nil{if x,ok:=e.(*exec.ExitError);ok{os.Exit(x.ExitCode())};panic(e)}}
