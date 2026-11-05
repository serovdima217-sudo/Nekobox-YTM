package io.nekohasekai.sagernet.linux
import java.net.*
import java.nio.ByteBuffer
import java.security.SecureRandom

/** Настоящие Binding/CHANGE-REQUEST, RFC 5389/5780. Отсутствие расширения не выдаётся за результат. */
object LinuxStun {
 data class Response(val mapped:InetSocketAddress?,val other:InetSocketAddress?,val origin:InetSocketAddress?)
 private const val cookie=0x2112A442
 fun decode(data:ByteArray,id:ByteArray):Response?{
  if(data.size<20||!data.copyOfRange(8,20).contentEquals(id))return null
  val b=ByteBuffer.wrap(data);if((b.short.toInt()and 0xffff)!=0x0101)return null
  val length=b.short.toInt()and 0xffff;if(b.int!=cookie||length+20>data.size)return null
  b.position(20);var mapped:InetSocketAddress?=null;var other:InetSocketAddress?=null;var origin:InetSocketAddress?=null
  while(b.position()+4<=length+20){val type=b.short.toInt()and 0xffff;val size=b.short.toInt()and 0xffff;val start=b.position();if(start+size>length+20)return null
   if(type in listOf(0x0020,0x0001,0x802c,0x0005,0x802b)&&size>=8){b.get();val family=b.get().toInt();var port=b.short.toInt()and 0xffff;val count=if(family==1)4 else if(family==2)16 else 0
    if(count>0&&size>=4+count){val address=ByteArray(count);b.get(address);if(type==0x0020){port=port xor(cookie ushr 16);val mask=ByteBuffer.allocate(16).putInt(cookie).put(id).array();for(i in address.indices)address[i]=(address[i].toInt()xor mask[i].toInt()).toByte()}
     val endpoint=InetSocketAddress(InetAddress.getByAddress(address),port);when(type){0x0020->mapped=endpoint;0x0001->if(mapped==null)mapped=endpoint;0x802c,0x0005->other=endpoint;0x802b->origin=endpoint}
    }
   };b.position(start+((size+3)/4)*4)
  };return Response(mapped,other,origin)
 }
 private fun request(socket:DatagramSocket,to:InetSocketAddress,change:Int=0):Response?{
  val id=ByteArray(12).also{SecureRandom().nextBytes(it)};val buffer=ByteBuffer.allocate(if(change==0)20 else 28).putShort(1).putShort(if(change==0)0.toShort()else 8.toShort()).putInt(cookie).put(id)
  if(change!=0)buffer.putShort(3).putShort(4).putInt(change)
  repeat(2){socket.send(DatagramPacket(buffer.array(),buffer.capacity(),to));val end=System.nanoTime()+2_000_000_000L
   while(System.nanoTime()<end){try{socket.soTimeout=((end-System.nanoTime())/1_000_000).toInt().coerceIn(1,2000);val packet=DatagramPacket(ByteArray(2048),2048);socket.receive(packet);decode(packet.data.copyOf(packet.length),id)?.let{return it}}catch(_:SocketTimeoutException){break}}
  };return null
 }
 fun test(server:String):String{
  val uri=URI("stun://"+server.trim());val host=uri.host?:error("Укажите STUN-сервер в формате сервер:порт");val endpoint=InetSocketAddress(host,if(uri.port>0)uri.port else 3478)
  return DatagramSocket().use{s->val first=request(s,endpoint)?:error("STUN-сервер не ответил за 4 секунды");val mapped=first.mapped?:error("Сервер не сообщил внешний адрес")
   buildString{append("Внешний адрес: ${mapped.address.hostAddress}:${mapped.port}\n");val other=first.other
    if(other==null)append("Сервер не поддерживает OTHER-ADDRESS: поведение NAT по RFC 5780 не определено.")
    else{
     val second=request(s,InetSocketAddress(other.address,endpoint.port));val mapping=if(second?.mapped==mapped)"Независимое от конечной точки"else if(second==null)"Не определено: нет ответа другого адреса"else{val third=request(s,other);if(third==null)"Не определено: нет ответа другого порта"else if(third.mapped==second.mapped)"Зависит от адреса"else"Зависит от адреса и порта"}
     val filtering=if(request(s,endpoint,6)!=null)"Независимая от конечной точки"else if(request(s,endpoint,2)!=null)"Зависит от адреса"else"Зависит от адреса и порта (либо CHANGE-REQUEST блокируется)"
     append("Сопоставление: $mapping\nФильтрация: $filtering")
    }
   }
  }
 }
}
