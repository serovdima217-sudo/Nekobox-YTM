package io.nekohasekai.sagernet.linux

import androidx.compose.runtime.*
import androidx.compose.foundation.*
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.*
import androidx.compose.material.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.fmt.AbstractBean
import io.nekohasekai.sagernet.fmt.internal.ChainBean

class ChainLinuxEditor:LinuxEditor {
 override fun load(bean:AbstractBean){with(bean as ChainBean){DataStore.profileCacheStore.putString("profileName",name);DataStore.profileCacheStore.putString("linuxChainProfiles",proxies.joinToString(","))}}
 override fun save(bean:AbstractBean){with(bean as ChainBean){name=DataStore.profileCacheStore.getString("profileName","");proxies=DataStore.profileCacheStore.getString("linuxChainProfiles","").orEmpty().split(',').mapNotNull{it.toLongOrNull()}}}
}
@Composable fun AndroidChainFields(entity:ProxyEntity,modifier:Modifier=Modifier) {
 var name by remember{mutableStateOf(DataStore.profileCacheStore.getString("profileName","").orEmpty())}
 var ids by remember{mutableStateOf(DataStore.profileCacheStore.getString("linuxChainProfiles","").orEmpty().split(',').mapNotNull{it.toLongOrNull()})}
 var add by remember{mutableStateOf(false)}
 val profiles=remember{SagerDatabase.proxyDao.getAll().filter{it.id!=entity.id}}
 fun change(next:List<Long>){ids=next;DataStore.profileCacheStore.putString("linuxChainProfiles",next.joinToString(","))}
 Column(modifier.fillMaxWidth()){
  OutlinedTextField(name,{name=it;DataStore.profileCacheStore.putString("profileName",it)},Modifier.fillMaxWidth().padding(16.dp),label={Text("Имя профиля")},singleLine=true)
  Text("Порядок цепочки: приложение → первый профиль → … → последний профиль → Интернет.",Modifier.padding(horizontal=16.dp))
  LazyColumn(Modifier.weight(1f)){
   itemsIndexed(ids,key={_,id->id}){index,id->Card(Modifier.fillMaxWidth().padding(horizontal=8.dp,vertical=4.dp)){
    Row(Modifier.padding(8.dp),verticalAlignment=Alignment.CenterVertically){
     Text("${index+1}. ${profiles.find{it.id==id}?.displayName()?:"Удалённый профиль $id"}",Modifier.weight(1f))
     TextButton(onClick={change(ids.toMutableList().apply{add(index-1,removeAt(index))})},enabled=index>0){Text("↑")}
     TextButton(onClick={change(ids.toMutableList().apply{add(index+1,removeAt(index))})},enabled=index<ids.lastIndex){Text("↓")}
     TextButton(onClick={change(ids-id)}){Text("×")}
    }
   }}
  }
  TextButton(onClick={add=true},modifier=Modifier.padding(8.dp)){Text("Добавить профиль")}
 }
 if(add)AlertDialog(onDismissRequest={add=false},title={Text("Добавить в цепочку")},text={
  LazyColumn(Modifier.heightIn(max=400.dp)){items(profiles.filter{it.id !in ids},key={it.id}){p->TextButton(onClick={change(ids+p.id);add=false},modifier=Modifier.fillMaxWidth()){Text(p.displayName())}}}
 },confirmButton={},dismissButton={TextButton(onClick={add=false}){Text("Отмена")}})
}
