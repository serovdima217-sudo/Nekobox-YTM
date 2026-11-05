package io.nekohasekai.sagernet.linux

import java.awt.*
import java.awt.event.*
import javax.imageio.ImageIO
import javax.swing.SwingUtilities

/** Системный трей Linux/KDE. Закрытие окна не завершает работающий VPN. */
class DesktopTray(private val show:()->Unit,private val toggle:()->Unit,private val quit:()->Unit,private val toggleWindow:()->Unit=show):AutoCloseable {
 @Volatile var ready=false;private set
 private var icon:TrayIcon?=null
 private var native:NativeTray?=null
 private val connect=MenuItem("Подключить")
 fun install():Boolean {
  if(GraphicsEnvironment.isHeadless())return false
  val candidate=NativeTray(show,toggle,quit,toggleWindow)
  if(candidate.install()){native=candidate;ready=true;return true}
  if(!SystemTray.isSupported())return false
  onEdt{
   val menu=PopupMenu();val open=MenuItem("Открыть Nekobox YTM").apply{addActionListener{show()}}
   menu.add(open);menu.addSeparator();menu.add(connect);connect.addActionListener{toggle()};menu.addSeparator();menu.add(MenuItem("Выход").apply{addActionListener{quit()}})
   val image=ImageIO.read(javaClass.getResourceAsStream("/android/icon/nekobox.png"))
   val tray=TrayIcon(image,"Nekobox YTM",menu).apply{isImageAutoSize=true;addMouseListener(object:MouseAdapter(){override fun mouseClicked(e:MouseEvent){if(e.button==MouseEvent.BUTTON1&&e.clickCount==1)toggleWindow()}})}
   SystemTray.getSystemTray().add(tray);icon=tray;ready=true
  }
  return ready
 }
 internal fun mouseAction(button:Int){native?.let{it.probe(if(button==MouseEvent.BUTTON1)"activate"else"menu");return};onEdt{val event=MouseEvent(Canvas(),MouseEvent.MOUSE_CLICKED,System.currentTimeMillis(),0,0,0,1,false,button);icon!!.mouseListeners.forEach{it.mouseClicked(event)}}}
 internal fun openAction(){native?.let{it.probe("open");return};onEdt{val item=icon!!.popupMenu.getItem(0);item.actionListeners.forEach{it.actionPerformed(ActionEvent(item,ActionEvent.ACTION_PERFORMED,item.label))}}}
 fun update(phase:String,selected:Boolean){native?.let{it.update(phase,selected);return};SwingUtilities.invokeLater{icon?.toolTip="Nekobox YTM — $phase";connect.label=if(phase=="Отключено")"Подключить"else"Отключить";connect.isEnabled=selected||phase!="Отключено"}}
 override fun close(){if(!ready)return;native?.let{it.close();native=null;ready=false;return};onEdt{icon?.let{SystemTray.getSystemTray().remove(it)};icon=null;ready=false}}
 private fun onEdt(action:()->Unit){if(SwingUtilities.isEventDispatchThread())action()else SwingUtilities.invokeAndWait(action)}
 companion object {
  fun test(){
   require(!GraphicsEnvironment.isHeadless()){ "Для трея нужна графическая сессия" }
   var shown=false;var toggled=false;var quit=false;var windowClicks=0
   val tray=DesktopTray({shown=true},{toggled=true},{quit=true},{windowClicks++})
   check(tray.install()){ "Окружение не предоставило системный трей" }
   if(tray.native!=null){
    val n=tray.native!!;println(n.probe("menu"));n.probe("open");n.probe("toggle");n.probe("exit")
    tray.mouseAction(MouseEvent.BUTTON1);tray.mouseAction(MouseEvent.BUTTON1);tray.mouseAction(MouseEvent.BUTTON3)
    tray.onEdt{};check(shown&&toggled&&quit&&windowClicks==2)
    println("Проверен нативный KDE StatusNotifierItem, настоящее меню получено через D-Bus, действия вызваны по D-Bus")
   }else{
    check(SystemTray.getSystemTray().trayIcons.isNotEmpty())
    tray.onEdt{val menu=tray.icon!!.popupMenu;for(i in listOf(0,2,4)){val item=menu.getItem(i);item.actionListeners.forEach{it.actionPerformed(ActionEvent(item,ActionEvent.ACTION_PERFORMED,item.label))}}}
    tray.mouseAction(MouseEvent.BUTTON1);tray.mouseAction(MouseEvent.BUTTON1);tray.mouseAction(MouseEvent.BUTTON3)
    check(shown&&toggled&&quit&&windowClicks==2)
   }
   tray.close();check(!tray.ready)
   println("Системный трей: значок зарегистрирован, открыть/подключить/выход обработаны, ЛКМ переключает окно, ПКМ не переключает и имеет меню; значок удалён")
  }
 }
}
