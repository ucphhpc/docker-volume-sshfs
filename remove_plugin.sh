docker volume rm -f $(docker volume ls -q)
docker plugin disable rasmunk/sshfs:latest
docker plugin rm -f rasmunk/sshfs:latest
