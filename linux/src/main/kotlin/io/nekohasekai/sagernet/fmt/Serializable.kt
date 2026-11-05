package io.nekohasekai.sagernet.fmt

import com.esotericsoftware.kryo.io.ByteBufferInput
import com.esotericsoftware.kryo.io.ByteBufferOutput

/** Android Parcel заменён тем же двоичным форматом Kryo и обычным JVM-фабричным интерфейсом. */
abstract class Serializable {
    abstract fun initializeDefaultValues()
    abstract fun serializeToBuffer(output: ByteBufferOutput)
    abstract fun deserializeFromBuffer(input: ByteBufferInput)
    interface Creator<T : Serializable> {
        fun newInstance(): T
        fun newArray(size: Int): Array<T?>
    }
    abstract class CREATOR<T : Serializable> : Creator<T>
}
