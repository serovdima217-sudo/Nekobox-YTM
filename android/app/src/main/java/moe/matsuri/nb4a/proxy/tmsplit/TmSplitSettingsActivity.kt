package moe.matsuri.nb4a.proxy.tmsplit

import android.os.Bundle
import android.view.Menu
import android.widget.Toast
import androidx.preference.EditTextPreference
import androidx.preference.PreferenceFragmentCompat
import io.nekohasekai.sagernet.R
import io.nekohasekai.sagernet.database.DataStore
import io.nekohasekai.sagernet.ktx.onMainDispatcher
import io.nekohasekai.sagernet.ui.profile.ProfileSettingsActivity
import moe.matsuri.nb4a.proxy.config.ConfigBean
import org.json.JSONArray
import org.json.JSONObject

class TmSplitSettingsActivity : ProfileSettingsActivity<ConfigBean>() {
    private val linksKey = "tmSplitLinks"
    override fun createEntity() = ConfigBean().apply { initializeDefaultValues(); type = 2 }
    override fun ConfigBean.init() {
        DataStore.profileName = name
        DataStore.profileCacheStore.putString(linksKey, tmSplitLinks())
    }
    private fun input(): ConfigBean {
        val raw = DataStore.profileCacheStore.getString(linksKey, "")!!.trim()
        val bean = if (raw.startsWith("tm-split://") || raw.startsWith("{")) parseTmSplit(raw)
        else parseTmSplit(JSONObject().put("type","tm-split").put("name",DataStore.profileName).put("links",JSONArray(raw.split(Regex("[\\s,;]+")).filter { it.isNotBlank() })).toString())
        if (DataStore.profileName.isNotBlank()) {
            bean.name=DataStore.profileName
            bean.config=JSONObject(bean.config).put("name",bean.name).toString()
        }
        return bean
    }
    override fun ConfigBean.serialize() {
        val bean=input();type=2;name=bean.name;config=bean.config
        initializeDefaultValues()
    }
    override suspend fun saveAndExit() {
        val problem=runCatching { input() }.exceptionOrNull()
        if (problem!=null) {
            onMainDispatcher { Toast.makeText(this@TmSplitSettingsActivity,problem.message,Toast.LENGTH_LONG).show() }
            return
        }
        super.saveAndExit()
    }
    override fun onCreateOptionsMenu(menu: Menu): Boolean {
        super.onCreateOptionsMenu(menu)
        menu.findItem(R.id.action_custom_outbound_json)?.isVisible=false
        menu.findItem(R.id.action_custom_config_json)?.isVisible=false
        return true
    }
    override fun PreferenceFragmentCompat.createPreferences(savedInstanceState: Bundle?, rootKey: String?) {
        addPreferencesFromResource(R.xml.tm_split_preferences)
        findPreference<EditTextPreference>(linksKey)?.setOnBindEditTextListener {
            it.isSingleLine=false
            it.minLines=6
            it.inputType=android.text.InputType.TYPE_CLASS_TEXT or android.text.InputType.TYPE_TEXT_FLAG_MULTI_LINE or android.text.InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS
        }
    }
}
