package io.nekohasekai.sagernet.database

import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.attribute.PosixFilePermissions
import java.sql.DriverManager
import java.lang.reflect.Proxy
import java.lang.reflect.Method
import moe.matsuri.nb4a.utils.JavaUtil.gson

/** Независимый SQLite-файл. Старые Android/Throne-базы никогда не открываются на запись. */
object LinuxDatabase {
 val directory: Path = Path.of(System.getenv("NEKOBOX_LINUX_STATE") ?: ((System.getenv("XDG_CONFIG_HOME") ?: (System.getProperty("user.home") + "/.config")) + "/NekoboxYTM-Android-Linux"))
 private val connection by lazy {
  Files.createDirectories(directory)
  Files.setPosixFilePermissions(directory,PosixFilePermissions.fromString("rwx------"))
  DriverManager.getConnection("jdbc:sqlite:"+directory.resolve("nekobox.db")).also { c ->
   c.createStatement().use { it.execute("PRAGMA journal_mode=WAL");it.execute("PRAGMA busy_timeout=5000");it.execute("CREATE TABLE IF NOT EXISTS documents(kind TEXT NOT NULL,id INTEGER NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(kind,id))");it.execute("CREATE TABLE IF NOT EXISTS preferences(namespace TEXT NOT NULL,name TEXT NOT NULL,value TEXT,PRIMARY KEY(namespace,name))") }
   Files.setPosixFilePermissions(directory.resolve("nekobox.db"),PosixFilePermissions.fromString("rw-------"))
  }
 }
 @Synchronized fun <T> all(kind:String, type:Class<T>):List<T> = connection.prepareStatement("SELECT payload FROM documents WHERE kind=? ORDER BY id").use { q -> q.setString(1,kind);q.executeQuery().use { r -> buildList { while(r.next())add(gson.fromJson(r.getString(1),type)) } } }
 @Synchronized fun put(kind:String, entity:Any):Long {
  val idField=entity.javaClass.getDeclaredField("id").also { it.isAccessible=true }
  var id=idField.getLong(entity)
  if(id==0L) {
   id=connection.prepareStatement("SELECT COALESCE(MAX(id),0)+1 FROM documents WHERE kind=?").use { q -> q.setString(1,kind);q.executeQuery().use { r -> r.next();r.getLong(1) } }
   idField.setLong(entity,id)
  }
  connection.prepareStatement("INSERT INTO documents(kind,id,payload) VALUES(?,?,?) ON CONFLICT(kind,id) DO UPDATE SET payload=excluded.payload").use { q -> q.setString(1,kind);q.setLong(2,id);q.setString(3,gson.toJson(entity));q.executeUpdate() }
  return id
 }
 @Synchronized fun delete(kind:String,ids:List<Long>):Int {
  var count=0
  connection.prepareStatement("DELETE FROM documents WHERE kind=? AND id=?").use { q -> for(id in ids){ q.setString(1,kind);q.setLong(2,id);count+=q.executeUpdate() } }
  return count
 }
 @Synchronized fun getPreference(namespace:String,name:String):String? = connection.prepareStatement("SELECT value FROM preferences WHERE namespace=? AND name=?").use { q -> q.setString(1,namespace);q.setString(2,name);q.executeQuery().use { r -> if(r.next())r.getString(1) else null } }
 @Synchronized fun setPreference(namespace:String,name:String,value:String?) {
  connection.prepareStatement("INSERT INTO preferences(namespace,name,value) VALUES(?,?,?) ON CONFLICT(namespace,name) DO UPDATE SET value=excluded.value").use { q -> q.setString(1,namespace);q.setString(2,name);q.setString(3,value);q.executeUpdate() }
 }
 @Synchronized fun preferences(namespace:String):Map<String,String> = connection.prepareStatement("SELECT name,value FROM preferences WHERE namespace=? AND value IS NOT NULL").use{q->q.setString(1,namespace);q.executeQuery().use{r->buildMap{while(r.next())put(r.getString(1),r.getString(2))}}}
 @Synchronized fun clearPreferences(namespace:String){connection.prepareStatement("DELETE FROM preferences WHERE namespace=?").use{q->q.setString(1,namespace);q.executeUpdate()}}
 @Synchronized fun <T> transaction(block:()->T):T {
  val old=connection.autoCommit;if(!old)return block();connection.autoCommit=false
  try { val result=block();connection.commit();return result } catch(t:Throwable){connection.rollback();throw t}finally{connection.autoCommit=old}
 }
 fun <D,T:Any> dao(kind:String,inter:Class<D>,type:Class<T>):D {
  fun field(o:T,key:String):Any? = type.getDeclaredField(key).also { it.isAccessible=true }.get(o)
  fun id(o:T)=(field(o,"id")as Number).toLong()
  fun number(o:T,key:String)=(field(o,key)as Number).toLong()
  @Suppress("UNCHECKED_CAST")
  return Proxy.newProxyInstance(inter.classLoader,arrayOf(inter)) { _,method,args0 ->
   val args=args0?:emptyArray();val name=method.name
   if(name=="toString")return@newProxyInstance "SQLite DAO: $kind"
   if(name=="hashCode")return@newProxyInstance kind.hashCode()
   val list=all(kind,type).sortedBy { runCatching{number(it,"userOrder")}.getOrDefault(id(it)) }
   val first=args.firstOrNull()
   fun returnCount(n:Int):Any? = if(method.returnType==Void.TYPE)null else if(method.returnType==java.lang.Long.TYPE)n.toLong() else n
   when {
    name in setOf("getAll","allGroups","allRules") -> list
    name=="subscriptions" -> list.filter{number(it,"type")==1L}
    name=="getById" -> list.find{id(it)==first}
    name=="getByGroup" -> list.filter{number(it,"groupId")==first}
    name=="getIdsByGroup" -> list.filter{number(it,"groupId")==first}.map{ id(it) }
    name=="getEntities" -> list.filter{id(it) in (first as List<*>)}
    name=="countByGroup" -> list.count{number(it,"groupId")==first}.toLong()
    name=="nextOrder" -> (list.filter{first==null||number(it,"groupId")==first}.maxOfOrNull { number(it,"userOrder") }?:-1L)+1L
    name=="enabledRules" -> list.filter{field(it,"enabled")== (first?:true)}
    name=="checkVpnNeeded" -> list.filter{field(it,"enabled")==true && (field(it,"packages")as Set<*>).isNotEmpty()}
    name=="reset" -> {delete(kind,list.map{ id(it) });null}
    name.startsWith("delete") -> {
     val ids=when {
      name=="deleteById" -> listOf(first as Long)
      name in setOf("deleteByGroup","deleteAll") -> list.filter { number(it,"groupId") in (if(first is LongArray)first.toList()else listOf(first)) }.map { id(it) }
      first is List<*> -> first.filterNotNull().map{id(it as T)}
      else -> listOf(id(first as T))
     }
     returnCount(delete(kind,ids))
    }
    name.startsWith("update")||name=="insert"||name=="addProxy"||name=="createGroup"||name=="createRule" -> {
     val values=if(first is List<*>)first.filterNotNull() else listOf(first!!)
     var last=0L;transaction { for(value in values)last=put(kind,value) }
     when(method.returnType){java.lang.Long.TYPE->last;java.lang.Integer.TYPE->values.size;else->null}
    }
    else -> throw UnsupportedOperationException("Не перенесён метод SQLite: $kind.$name")
   }
  } as D
 }
}
object SagerDatabase {
 val proxyDao = LinuxDatabase.dao("profiles",ProxyEntity.Dao::class.java,ProxyEntity::class.java)
 val groupDao = LinuxDatabase.dao("groups",ProxyGroup.Dao::class.java,ProxyGroup::class.java)
 val rulesDao = LinuxDatabase.dao("rules",RuleEntity.Dao::class.java,RuleEntity::class.java)
 fun <T> runInTransaction(block:()->T):T = LinuxDatabase.transaction(block)
}
class PreferenceStore(private val namespace:String) {
 fun getString(key:String,default:String?=null):String? = LinuxDatabase.getPreference(namespace,key)?:default
 fun putString(key:String,value:String?) = LinuxDatabase.setPreference(namespace,key,value)
 fun getInt(key:String,default:Int=0):Int = getString(key)?.toIntOrNull()?:default
 fun putInt(key:String,value:Int)=putString(key,value.toString())
 fun getLong(key:String,default:Long=0):Long = getString(key)?.toLongOrNull()?:default
 fun putLong(key:String,value:Long)=putString(key,value.toString())
 fun getBoolean(key:String,default:Boolean=false):Boolean = getString(key)?.toBooleanStrictOrNull()?:default
 fun putBoolean(key:String,value:Boolean)=putString(key,value.toString())
 fun getStringSet(key:String,default:Set<String> = emptySet()):Set<String> = getString(key)?.let{gson.fromJson(it,Array<String>::class.java).toSet()}?:default
 fun putStringSet(key:String,value:Set<String>)=putString(key,gson.toJson(value))
 fun remove(key:String)=putString(key,null)
}
