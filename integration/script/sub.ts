import { createPublicClient, webSocket} from "viem"

const chains = ["hyperevm"]
//const chains = ["polygon"]

const venn_url = process.env.VENN_URL || `localhost:8545`

const provider = Object.fromEntries(chains.map((x)=>{
  const tp = webSocket(`ws://${venn_url}/${x}`)
  const pc = createPublicClient({
    transport:tp,
  })
  return [x, pc]
}))

chains.forEach((n)=>{
  const subClient = provider[n].transport.subscribe({
    params:["newHeads"],
    onData: (x)=>{
      console.log(n, x.result.number, new Date())
    }
  }).catch(console.error)
})
