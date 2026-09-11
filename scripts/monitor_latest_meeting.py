import paramiko, json

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)

def run(cmd):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=120)
    return stdout.read().decode().strip(), stderr.read().decode().strip()

# latest meeting
q = 'db.meetings.find().sort({created_at:-1}).limit(1).toArray().map(m=>({id:m._id,title:m.title,status:m.status,created_at:m.created_at,creator:m.creator_id,participants:(m.participants||[]).map(p=>({name:p.agent_name,id:p.agent_id,status:p.status})),min:!!m.minutes_file_id}))'
out, err = run("docker exec trustmesh-mongo mongosh trustmesh --quiet --eval \"%s\"" % q)
print("=== latest meeting ===")
print(out)
print("ERR:", err)

# extract id
import re
m = re.search(r"id: '([^']+)'", out)
MID = m.group(1) if m else None
print("\nMID =", MID)

if MID:
    q2 = ("db.meeting_messages.find({meeting_id:'%s'}).sort({_id:1}).toArray()"
          ".map(x=>({sender:x.sender_name||x.sender_id,type:x.sender_type,content:(x.content||'').slice(0,160)}))") % MID
    out2, err2 = run("docker exec trustmesh-mongo mongosh trustmesh --quiet --eval \"%s\"" % q2)
    print("\n=== meeting messages ===")
    print(out2)
    print("ERR:", err2)

    # per-agent counts (real content, exclude pure ACK)
    q3 = ("db.meeting_messages.aggregate([{'$match':{meeting_id:'%s'}},"
          "{'$group':{_id:'$sender_name',n:{$sum:1}}},{$sort:{n:-1}}])") % MID
    out3, err3 = run("docker exec trustmesh-mongo mongosh trustmesh --quiet --eval \"%s\"" % q3)
    print("\n=== per-sender counts ===")
    print(out3)
    print("ERR:", err3)

ssh.close()
