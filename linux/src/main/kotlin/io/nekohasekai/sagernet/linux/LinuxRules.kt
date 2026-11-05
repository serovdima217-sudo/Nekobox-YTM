package io.nekohasekai.sagernet.linux

import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.ktx.app

object LinuxRules {
 /** Шаблоны исходного ProfileManager. Не включаются без выбора пользователя. */
 fun reset(){SagerDatabase.runInTransaction{
  SagerDatabase.rulesDao.reset()
  val rules=mutableListOf(RuleEntity(name=app.getString("route_opt_block_quic"),port="443",network="udp",outbound=-2),RuleEntity(name=app.getString("route_opt_block_ads"),domains="geosite:category-ads-all",outbound=-2))
  for((country,label)in listOf("cn" to "中国","ir" to "Iran","ru" to "Russia")){
   if(country=="cn")rules+=RuleEntity(name=app.getString("route_play_store",label),domains="googleapis.cn")
   rules+=RuleEntity(name=app.getString("route_bypass_domain",label),domains="geosite:$country",outbound=-1)
   rules+=RuleEntity(name=app.getString("route_bypass_ip",label),ip="geoip:$country",outbound=-1)
  }
  rules.forEachIndexed{i,r->r.userOrder=i.toLong();r.enabled=false;SagerDatabase.rulesDao.createRule(r)}
  DataStore.rulesFirstCreate=true
 }}
}
