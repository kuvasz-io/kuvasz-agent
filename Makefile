kuvasz-agent:
	go build

check:
	trunk check

install: 
	sudo systemctl stop kuvasz-agent
	sudo cp kuvasz-agent /usr/sbin
	sudo systemctl start kuvasz-agent

clean:
	rm -f kuvasz-agent *.rpm *.deb

package: kuvasz-agent kuvasz-agent.ini 
	rm -rf packaging/deb packaging/rpm
	rm -f kuvasz-agent-*.rpm
	mkdir -p packaging/rpm/etc/sysconfig
	mkdir -p packaging/rpm/etc/init.d
	mkdir -p packaging/rpm/etc/kuvasz
	mkdir -p packaging/rpm/usr/sbin
	mkdir -p packaging/rpm/usr/lib/systemd/system
	cp packaging/rpm.etc.sysconfig.kuvasz-agent packaging/rpm/etc/sysconfig/kuvasz-agent
	cp kuvasz-agent.ini                         packaging/rpm/etc/kuvasz
	cp kuvasz-agent                             packaging/rpm/usr/sbin
	cp packaging/rpm.etc.init.d.kuvasz-agent    packaging/rpm/etc/init.d/kuvasz-agent
	cp packaging/rpm.kuvasz-agent.service       packaging/rpm/usr/lib/systemd/system/kuvasz-agent.service
	fpm -s dir -t rpm -n kuvasz-agent -v 15 --iteration 2 --config-files /etc/kuvasz/kuvasz-agent.ini --config-files /usr/lib/systemd/system/kuvasz-agent.service --license Proprietary --vendor Kuvasz.io -m help@kuvasz.io --description "Kuvasz agent" --url "http://kuvasz.io" --after-install packaging/postinst.rpm -C packaging/rpm .
	rm -f kuvasz-agent_*.deb
	mkdir -p packaging/deb/etc/default
	mkdir -p packaging/deb/etc/init.d
	mkdir -p packaging/deb/etc/kuvasz
	mkdir -p packaging/deb/usr/sbin
	mkdir -p packaging/deb/lib/systemd/system
	cp kuvasz-agent.ini                        packaging/deb/etc/kuvasz
	cp kuvasz-agent                            packaging/deb/usr/sbin
	cp packaging/deb.etc.init.d.kuvasz-agent   packaging/deb/etc/init.d/kuvasz-agent
	cp packaging/deb.kuvasz-agent.service      packaging/deb/lib/systemd/system/kuvasz-agent.service
	cp packaging/deb.etc.default.kuvasz-agent  packaging/deb/etc/default/kuvasz-agent
	fpm -s dir -t deb -n kuvasz-agent -v 15 --epoch 2 --config-files /etc/kuvasz/kuvasz-agent.ini --license Proprietary --vendor Kuvasz.io -m help@kuvasz.io --description "Kuvasz agent" --url "http://kuvasz.io" --after-install packaging/postinst.deb -C packaging/deb .
	rm -rf packaging/rpm packaging/deb


